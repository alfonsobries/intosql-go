package intosql

// The conversion contract, written by hand from the engine's contract (the TypeScript SDK's
// contract.generated.ts). Enum types are plain strings, so values added later still decode.

// ContractVersion is the version of the conversion spec and report this package speaks.
const ContractVersion = 1

// Dialect is a database IntoSQL writes SQL for.
type Dialect string

// The dialects IntoSQL writes SQL for.
const (
	DialectPostgreSQL Dialect = "postgresql"
	DialectMySQL      Dialect = "mysql"
	DialectMariaDB    Dialect = "mariadb"
	DialectSQLite     Dialect = "sqlite"
	DialectSQLServer  Dialect = "sql_server"
	DialectOracle     Dialect = "oracle"
)

// OutputFormat is what a conversion writes.
type OutputFormat string

// The output formats.
const (
	FormatSQL  OutputFormat = "sql"
	FormatCSV  OutputFormat = "csv"
	FormatJSON OutputFormat = "json"
)

// OutputStyle is how rows are written.
type OutputStyle string

// The output styles.
const (
	StyleInserts OutputStyle = "inserts"
	StyleBulk    OutputStyle = "bulk"
)

// ColumnNames is how column names are written.
type ColumnNames string

// The ways column names are written.
const (
	ColumnNamesSnakeCase ColumnNames = "snake_case"
	ColumnNamesAsWritten ColumnNames = "as_written"
)

// DateOrder is how ambiguous dates such as 03/02/2024 are read.
type DateOrder string

// The date orders.
const (
	DateOrderDayFirst   DateOrder = "day_first"
	DateOrderMonthFirst DateOrder = "month_first"
	DateOrderYearFirst  DateOrder = "year_first"
	DateOrderAuto       DateOrder = "auto"
)

// InputFormat is what a file can be read as.
type InputFormat string

// The input formats.
const (
	InputXLSX    InputFormat = "xlsx"
	InputXLS     InputFormat = "xls"
	InputXLSB    InputFormat = "xlsb"
	InputODS     InputFormat = "ods"
	InputCSV     InputFormat = "csv"
	InputTSV     InputFormat = "tsv"
	InputJSON    InputFormat = "json"
	InputNDJSON  InputFormat = "ndjson"
	InputSQLDump InputFormat = "sql_dump"
)

// ColumnKind is the kind of value a column holds, before it is mapped to a dialect's type.
type ColumnKind string

// Severity is how serious a problem is: "error", "rejected" or "warning".
type Severity string

// ErrorCode is a problem the engine found: what stopped a conversion, left a row out or needs a
// look, such as "encrypted_file" or "invalid_date".
type ErrorCode string

// FixCode is a correction the engine made, such as "leading_zeros_kept".
type FixCode string

// ObjectKind is what a database dump defines besides tables, which a conversion lists and does
// not carry.
type ObjectKind string

// The objects of a dump that are not converted.
const (
	ObjectView      ObjectKind = "view"
	ObjectTrigger   ObjectKind = "trigger"
	ObjectProcedure ObjectKind = "procedure"
	ObjectFunction  ObjectKind = "function"
	ObjectEvent     ObjectKind = "event"
)

// JobSpec is a conversion request, the same for the browser, the CLI, the API and MCP.
type JobSpec struct {
	Version   int           `json:"version"`
	Input     InputSpec     `json:"input"`
	Output    OutputSpec    `json:"output"`
	Inference InferenceSpec `json:"inference"`
	// Changes to single columns; every other column is written as detected.
	Columns []ColumnSpec `json:"columns"`
}

// InputSpec is how to read the input. A nil field is detected from the file.
type InputSpec struct {
	Format    *InputFormat `json:"format,omitempty"`
	Sheet     *string      `json:"sheet"`
	HeaderRow *int         `json:"header_row"`
	Delimiter *string      `json:"delimiter"`
	Encoding  *string      `json:"encoding"`
}

// OutputSpec is what to write.
type OutputSpec struct {
	Format OutputFormat `json:"format"`
	// Decides names and types, for every format.
	Dialect Dialect `json:"dialect"`
	// Derived from the file or sheet name when nil.
	Table       *string     `json:"table"`
	Schema      *string     `json:"schema"`
	CreateTable bool        `json:"create_table"`
	ColumnNames ColumnNames `json:"column_names"`
	Style       OutputStyle `json:"style"`
	// Rows per statement or bulk batch.
	BatchRows int `json:"batch_rows"`
}

// InferenceSpec is how column types are inferred.
type InferenceSpec struct {
	// Rows read before the types are decided.
	SampleRows int       `json:"sample_rows"`
	DateOrder  DateOrder `json:"date_order"`
	// Values such as 00127 stay text so the zeros survive.
	KeepLeadingZeros bool `json:"keep_leading_zeros"`
	// A BCP 47 tag such as es-MX.
	Locale *string `json:"locale"`
}

// ColumnSpec is what to do with one column of the file, found by its 0-based position.
type ColumnSpec struct {
	Index   int     `json:"index"`
	Name    *string `json:"name,omitempty"`
	AsText  bool    `json:"as_text,omitempty"`
	Exclude bool    `json:"exclude,omitempty"`
}

// JobReport is what a conversion did, the same on every surface.
type JobReport struct {
	Version  int            `json:"version"`
	Input    DetectedInput  `json:"input"`
	Dialect  Dialect        `json:"dialect"`
	Table    string         `json:"table"`
	Columns  []ColumnReport `json:"columns"`
	Rows     RowCounts      `json:"rows"`
	Fixes    []Fix          `json:"fixes"`
	Problems []Problem      `json:"problems"`
	Timings  Timings        `json:"timings"`
	// For a database dump: where it comes from. Nil for any other file.
	Source *SourceReport `json:"source,omitempty"`
	// For a database dump: every table, with its own columns, fixes and problems. The fields
	// above then hold the totals and what concerns the whole dump.
	Tables []TableReport `json:"tables,omitempty"`
	// For a database dump: the views, triggers, routines and events it did not carry.
	NotConverted []NotConverted `json:"not_converted,omitempty"`
}

// SourceReport is where a database dump comes from.
type SourceReport struct {
	// The database that wrote the dump, from its header and syntax.
	Dialect Dialect `json:"dialect"`
	// The server version the dump states, such as 8.0.40.
	Version *string `json:"version,omitempty"`
	// The program that wrote it, such as mysqldump or phpMyAdmin.
	Producer *string `json:"producer,omitempty"`
	// The character set the dump declares, or the one read from its bytes.
	Charset *string `json:"charset,omitempty"`
	// The databases the dump holds, in order.
	Databases []string `json:"databases"`
}

// TableReport is one table of a database dump, as read and as written.
type TableReport struct {
	SourceName string `json:"source_name"`
	Name       string `json:"name"`
	// The schema it goes in, when not the default one.
	Schema *string `json:"schema,omitempty"`
	// Rows in the dump; rows written when converted.
	Rows    int64          `json:"rows"`
	Columns []ColumnReport `json:"columns"`
	// Fixes about this table and its columns, numbered across the whole report.
	Fixes    []Fix     `json:"fixes"`
	Problems []Problem `json:"problems"`
}

// NotConverted is an object of a dump that the conversion did not carry: it is recreated by hand.
type NotConverted struct {
	Kind ObjectKind `json:"kind"`
	// The name in the dump, with its database when the dump names more than one.
	Name string `json:"name"`
}

// DetectedInput is what was detected about the input.
type DetectedInput struct {
	Format    InputFormat `json:"format"`
	Encoding  string      `json:"encoding"`
	Delimiter *string     `json:"delimiter,omitempty"`
	Sheet     *string     `json:"sheet,omitempty"`
	Bytes     int64       `json:"bytes"`
}

// ColumnReport is one column as it was read and as it was written.
type ColumnReport struct {
	SourceName string     `json:"source_name"`
	Name       string     `json:"name"`
	SQLType    string     `json:"sql_type"`
	Kind       ColumnKind `json:"kind"`
	Precision  *int       `json:"precision,omitempty"`
	Scale      *int       `json:"scale,omitempty"`
	MaxChars   *int       `json:"max_chars,omitempty"`
	Nullable   bool       `json:"nullable"`
	Fixes      []int      `json:"fixes"`
	Excluded   bool       `json:"excluded,omitempty"`
	AsText     bool       `json:"as_text,omitempty"`
}

// RowCounts are the row totals of a conversion.
type RowCounts struct {
	Read      int64 `json:"read"`
	Converted int64 `json:"converted"`
	Rejected  int64 `json:"rejected"`
}

// Fix is a correction the engine made, numbered like a proofreader's mark.
type Fix struct {
	Number int               `json:"number"`
	Code   FixCode           `json:"code"`
	Column *string           `json:"column,omitempty"`
	Row    *int64            `json:"row,omitempty"`
	Args   map[string]string `json:"args"`
}

// Problem is something that stopped the conversion, left a row out or needs a look.
type Problem struct {
	Code     ErrorCode         `json:"code"`
	Severity Severity          `json:"severity"`
	Column   *string           `json:"column,omitempty"`
	Row      *int64            `json:"row,omitempty"`
	Args     map[string]string `json:"args"`
}

// Timings is how long each stage took.
type Timings struct {
	ReadMS    int64 `json:"read_ms"`
	ConvertMS int64 `json:"convert_ms"`
	TotalMS   int64 `json:"total_ms"`
}
