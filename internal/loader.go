package internal

import (
	"fmt"
	"os"
	"strings"

	"github.com/gedex/inflector"
	"github.com/knq/snaker"
	"go.mercari.io/yo/models"
	"gopkg.in/yaml.v2"
)

type loaderImpl interface {
	ParamN(int) string
	MaskFunc() string
	ParseType(string, bool) (int, string, string)
	ValidCustomType(string, string) bool
	TableList() ([]*models.Table, error)
	ColumnList(string) ([]*models.Column, error)
	IndexList(string) ([]*models.Index, error)
	IndexColumnList(string, string) ([]*models.IndexColumn, error)
}

func NewTypeLoader(l loaderImpl) *TypeLoader {
	return &TypeLoader{loader: l}
}

// TypeLoader provides a common Loader implementation used by the built in
// schema/query loaders.
type TypeLoader struct {
	CustomTypes *models.CustomTypes
	loader      loaderImpl
}

// NthParam satisifies Loader's NthParam.
func (tl *TypeLoader) NthParam(i int) string {
	return tl.loader.ParamN(i)
}

// Mask returns the parameter mask.
func (tl *TypeLoader) Mask() string {

	return tl.loader.MaskFunc()
}

// LoadSchema loads schema definitions.
func (tl *TypeLoader) LoadSchema(args *ArgType) (map[string]*Type, map[string]*Index, error) {
	var err error

	// load tables
	tableMap, err := tl.LoadTable(args)
	if err != nil {
		return nil, nil, err
	}

	// load indexes
	ixMap, err := tl.LoadIndexes(args, tableMap)
	if err != nil {
		return nil, nil, err
	}

	return tableMap, ixMap, nil
}

// LoadTable loads a schema table/view definition.
func (tl *TypeLoader) LoadTable(args *ArgType) (map[string]*Type, error) {
	var err error

	// load tables
	tableList, err := tl.loader.TableList()
	if err != nil {
		return nil, err
	}

	// tables
	tableMap := make(map[string]*Type)
	for _, ti := range tableList {
		ignore := false

		for _, ignoreTable := range args.IgnoreTables {
			if ignoreTable == ti.TableName {
				// Skip adding this table if user has specified they are not
				// interested.
				//
				// This could be useful for tables which are managed by the
				// database (e.g. SchemaMigrations) instead of
				// via Go code.
				ignore = true
			}
		}

		if ignore {
			continue
		}

		// create template
		typeTpl := &Type{
			Name:   SingularizeIdentifier(ti.TableName),
			Schema: "",
			Fields: []*Field{},
			Table:  ti,
		}

		// process columns
		err = tl.LoadColumns(args, typeTpl)
		if err != nil {
			return nil, err
		}

		tableMap[ti.TableName] = typeTpl
	}

	return tableMap, nil
}

// tableCustomTypes find custom type definitions of the table
func (tl *TypeLoader) tableCustomTypes(table string) map[string]string {
	var columnTypes map[string]string
	if tl.CustomTypes != nil {
		for _, v := range tl.CustomTypes.Tables {
			if v.Name == table {
				columnTypes = v.Columns
				break
			}
		}
	}

	return columnTypes
}

// LoadColumns loads schema table/view columns.
func (tl *TypeLoader) LoadColumns(args *ArgType, typeTpl *Type) error {
	var err error

	// load columns
	columnList, err := tl.loader.ColumnList(typeTpl.Table.TableName)
	if err != nil {
		return err
	}

	columnTypes := tl.tableCustomTypes(typeTpl.Table.TableName)
	// process columns
	for _, c := range columnList {
		ignore := false

		for _, ignoreField := range args.IgnoreFields {
			if ignoreField == c.ColumnName {
				// Skip adding this field if user has specified they are not
				// interested.
				//
				// This could be useful for fields which are managed by the
				// database (e.g. automatically updated timestamps) instead of
				// via Go code.
				ignore = true
			}
		}

		if ignore {
			continue
		}

		// set col info
		f := &Field{
			Name: snaker.ForceCamelIdentifier(c.ColumnName),
			// Name: c.ColumnName,
			Col: c,
		}

		f.Len, f.NilType, f.Type = tl.loader.ParseType(c.DataType, !c.NotNull)

		// set custom type
		if columnTypes != nil {
			if t, ok := columnTypes[c.ColumnName]; ok && tl.loader.ValidCustomType(c.DataType, t) {
				f.CustomType = t
			}
		}

		// set primary key
		if c.IsPrimaryKey {
			typeTpl.PrimaryKeyFields = append(typeTpl.PrimaryKeyFields, f)
			// This is retained for backward compatibility in the templates.
			typeTpl.PrimaryKey = f
		}

		// append col to template fields
		typeTpl.Fields = append(typeTpl.Fields, f)
	}

	return nil
}

// LoadIndexes loads schema index definitions.
func (tl *TypeLoader) LoadIndexes(args *ArgType, tableMap map[string]*Type) (map[string]*Index, error) {
	var err error

	ixMap := map[string]*Index{}
	for _, t := range tableMap {
		// load table indexes
		err = tl.LoadTableIndexes(args, t, ixMap)
		if err != nil {
			return nil, err
		}
	}

	return ixMap, nil
}

// LoadTableIndexes loads schema index definitions per table.
func (tl *TypeLoader) LoadTableIndexes(args *ArgType, typeTpl *Type, ixMap map[string]*Index) error {
	var err error
	var priIxLoaded bool

	// load indexes
	indexList, err := tl.loader.IndexList(typeTpl.Table.TableName)
	if err != nil {
		return err
	}

	// process indexes
	for _, ix := range indexList {
		// save whether or not the primary key index was processed
		priIxLoaded = priIxLoaded || ix.IsPrimary || (ix.Origin == "pk")

		// create index template
		ixTpl := &Index{
			Schema: "",
			Type:   typeTpl,
			Fields: []*Field{},
			Index:  ix,
		}

		// load index columns
		err = tl.LoadIndexColumns(args, ixTpl)
		if err != nil {
			return err
		}

		// build func name
		ixTpl.FuncName = tl.buildIndexFuncName(ixTpl)

		ixMap[typeTpl.Table.TableName+"_"+ix.IndexName] = ixTpl
	}

	// search for primary key if it was skipped being set in the type
	if typeTpl.PrimaryKey == nil {
		return fmt.Errorf("no primary key found for %v", typeTpl.Name)
	}

	return nil
}

func (tl *TypeLoader) buildIndexFuncName(ixTpl *Index) string {
	// build func name
	funcName := ixTpl.Type.Name
	if !ixTpl.Index.IsUnique {
		funcName = inflector.Pluralize(ixTpl.Type.Name)
	}
	funcName = funcName + "By"

	// add param names
	paramNames := make([]string, 0, len(ixTpl.Fields))
	for _, f := range ixTpl.Fields {
		paramNames = append(paramNames, f.Name)
	}

	return funcName + strings.Join(paramNames, "")
}

// LoadIndexColumns loads the index column information.
func (tl *TypeLoader) LoadIndexColumns(args *ArgType, ixTpl *Index) error {
	var err error

	// load index columns
	indexCols, err := tl.loader.IndexColumnList(ixTpl.Type.Table.TableName, ixTpl.Index.IndexName)
	if err != nil {
		return err
	}

	// process index columns
	for _, ic := range indexCols {
		var field *Field

	fieldLoop:
		// find field
		for _, f := range ixTpl.Type.Fields {
			if f.Col.ColumnName == ic.ColumnName {
				field = f
				break fieldLoop
			}
		}

		if field == nil {
			continue
		}

		ixTpl.Fields = append(ixTpl.Fields, field)
	}

	return nil
}

// LoadCustomTypes loads custom type definition from yml file
func (tl *TypeLoader) LoadCustomTypes(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	var ctypes models.CustomTypes
	if err := yaml.NewDecoder(file).Decode(&ctypes); err != nil {
		return err
	}

	tl.CustomTypes = &ctypes

	return nil
}
