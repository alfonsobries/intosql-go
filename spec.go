package intosql

// SpecOptions are the options of NewSpec. Only Dialect is required; zero values take IntoSQL's
// defaults.
type SpecOptions struct {
	Dialect Dialect
	// "sql" when empty.
	Format OutputFormat
	// Chosen by the engine when empty.
	Table string
	// None when empty.
	Schema string
	// The first sheet when empty.
	Sheet string
	// Leave out CREATE TABLE. The default writes it.
	SkipCreateTable bool
	// 500 when zero.
	BatchRows int
}

// NewSpec returns a conversion request with IntoSQL's defaults: types inferred, names in
// snake_case, batched inserts.
func NewSpec(options SpecOptions) JobSpec {
	format := options.Format
	if format == "" {
		format = FormatSQL
	}

	batchRows := options.BatchRows
	if batchRows == 0 {
		batchRows = 500
	}

	return JobSpec{
		Version: ContractVersion,
		Input:   InputSpec{Sheet: optional(options.Sheet)},
		Output: OutputSpec{
			Format:      format,
			Dialect:     options.Dialect,
			Table:       optional(options.Table),
			Schema:      optional(options.Schema),
			CreateTable: !options.SkipCreateTable,
			ColumnNames: ColumnNamesSnakeCase,
			Style:       StyleInserts,
			BatchRows:   batchRows,
		},
		Inference: InferenceSpec{
			SampleRows:       10_000,
			DateOrder:        DateOrderAuto,
			KeepLeadingZeros: true,
		},
		Columns: []ColumnSpec{},
	}
}

// optional is nil for an empty string, so it is sent as null.
func optional(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}
