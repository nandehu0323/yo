# yo

`yo` is a command-line tool to generate Go code for [Google Cloud Spanner](https://cloud.google.com/spanner/).
forked from [xo](https://github.com/xo/xo) :rose:

`yo` uses database schema to generate code by using [Information Schema](https://cloud.google.com/spanner/docs/information-schema). `yo` runs SQL queries against tables in `INFORMATION_SCHEMA` to fetch metadata for a database, and applies the metadata to Go templates to generate code/models to acccess Cloud Spanner.

## Installation

```sh
$ go get -u go.mercari.io/yo
```

## Quickstart

The following is a quick overview of using `yo` on the command-line:

```sh
# change to project directory
$ cd $GOPATH/src/path/to/project

# make an output directory
$ mkdir -p models

# generate code for a schema
$ yo $PROJECT_NAME $INSTANCE_NAME $DATABASE_NAME -o models
```

## Command Line Options

The following are `yo`'s command-line arguments and options:

```sh
$ yo --help
yo is a command-line tool to generate Go code for Google Cloud Spanner.

Usage:
  yo PROJECT_NAME INSTANCE_NAME DATABASE_NAME [flags]

Examples:
  # Generate models under models directory
  yo $PROJECT_NAME $INSTANCE_NAME $DATABASE_NAME -o models

  # Generate models under models directory with custom types
  yo $PROJECT_NAME $INSTANCE_NAME $DATABASE_NAME -o models --custom-types-file custom_column_types.yml

Flags:
      --custom-type-package string   Go package name to use for custom or unknown types
      --custom-types-file string     custom table field type definition file
  -h, --help                         help for yo
      --ignore-fields stringArray    fields to exclude from the generated Go code types
      --ignore-tables stringArray    tables to exclude from the generated Go code types
  -o, --out string                   output path or file name
  -p, --package string               package name used in generated Go code
      --single-file                  toggle single file output
      --suffix string                output file suffix (default ".yo.go")
      --tags string                  build tags to add to package header
      --template-path string         user supplied template path
```

## Genearated code

`yo` generates a file per a table by default. Each files has struct, metadata, methods for a table.

### struct

From this table definition:

```
CREATE TABLE Examples (
  PKey STRING(32) NOT NULL,
  Num INT64 NOT NULL,
  CreatedAt TIMESTAMP NOT NULL,
) PRIMARY KEY(PKey);
```

This struct is generated:

```golang
type Example struct {
	PKey      string    `spanner:"PKey" json:"PKey"`           // PKey
	Num       int64     `spanner:"Num" json:"Num"`             // Num
	CreatedAt time.Time `spanner:"CreatedAt" json:"CreatedAt"` // CreatedAt
}
```

### Mutation methods

An operation agaist a table is represented as mutation in Cloud Spanner. `yo` generates methods to create mutation to modify a table.

* Insert
   * A wrapper method of `spanner.Insert`, which embeds struct values implicitly to insert a new record with struct values.
* Update
   * A wrapper method of `spanner.Update`, which embeds struct values implicitly to update all columns into struct values.
* InsertOrUpdate
   * A wrapper method of `spanner.InsertOrUpdate`, which embeds struct values implicitly to insert a new record or update all columns to struct values.
* UpdateColumns
   * A wrapper method of `spanner.Update`, which updates specified columns into struct values.

### Read functions

`yo` generates functions to read data from Cloud Spanner. The functions are generated based on index.

Naming convention of genearted functions is `FindXXXByYYY`. The XXX is table name and YYY is index name. XXX will be singular if the index is unique index, or plural if the index is not unique.


**TODO**

* Generated functions use `Query` only even if it is secondary index. Need a function to use `Read`.

### Error handling

`yo` wraps all errors as internal `yoError`. It has some methods for error handling.

* `GRPCStatus()`
   * This returns gRPC's `*status.Status`. It is intended by used from status google.golang.org/grpc/status package.
* `DBTableName()`
   * A table name where the error happens.
* `NotFound()`
   * A helper to check the error is NotFound.

The `yoError` inherits an original error from [google-cloud-go](https://github.com/GoogleCloudPlatform/google-cloud-go). It stil can be used with `status.FromError` or `status.Code` to check status code of the error. So the typical error handling will be like:

```golang
result, err := SomeFunction()
if err != nil {
	code := status.Code(err)
	if code == codes.InvalidArgument {
		// error handling for invalid argument
	}
	...
	panic("unexpected")
}
```

## Templates for code generation

`yo` uses Go [template](https://golang.org/pkg/text/template/) package to generate code. You can use your own template for code generation by using `--template-path` option.

`yo` provides default templates and uses them when `--template-path` option is not specified. The templates exist in [templates](templates/) directory. The templates are embeded into `yo` binary.

### Custom Template Quickstart

The following is a quick overview of copying the base templates contained in
the `yo` project's [`templates/`](templates) directory, editing to suit, and
using with `yo`:

```sh
# change to working project directory
$ cd $GOPATH/src/path/to/my/project

# create a template directory
$ mkdir -p templates

# copy yo templates
$ cp "$GOPATH/src/go.mercari.io/yo/templates/*" templates/

# remove yo binary data
$ rm templates/*.go

# edit base templates
$ vi templates/*.tpl.go

# use with yo
$ yo $PROJECT_NAME $INSTANCE_NAME $DATABASE_NAME -o models --template-path templates
```

See the Custom Template example below for more information on adapting the base
templates in the `yo` source tree for use within your own project.

### Template files

| Template File                  | Description                                           |
|--------------------------------|-------------------------------------------------------|
| `templates/type.go.tpl`        | Template for schema tables                            |
| `templates/index.go.tpl`       | Template for schema indexes                           |
| `templates/yo_db.go.tpl`       | Package level template generated once per package     |
| `templates/yo_package.go.tpl`  | File header template generated once per file          |

### Helpers functions

**This is not a stable feature**

`yo` provides some helper functions which can be used in templates. Those are defined in [`generator/funcs.go`](generator/funcs.go). Those are not well documented and are likely to change.

## Contribution

Please read the CLA below carefully before submitting your contribution.

https://www.mercari.com/cla/

## License

Copyright 2018 Mercari, Inc.

yo is released under the [MIT License](https://opensource.org/licenses/MIT).
