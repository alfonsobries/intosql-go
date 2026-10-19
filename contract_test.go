package intosql

import (
	"encoding/json"
	"os"
	"testing"
)

func decodeReport(t *testing.T, data []byte) JobReport {
	t.Helper()

	var report JobReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("decode report: %v", err)
	}

	return report
}

func TestDecodesTheReportOfADatabaseDump(t *testing.T) {
	data, err := os.ReadFile("testdata/dump_report.json")
	if err != nil {
		t.Fatal(err)
	}

	report := decodeReport(t, data)

	equal(t, report.Input.Format, InputSQLDump)
	equal(t, report.Dialect, DialectPostgreSQL)
	equal(t, report.Rows.Converted, int64(18))
	equal(t, report.Problems[0].Code, ErrorCode("not_converted_objects"))

	source := report.Source
	if source == nil {
		t.Fatal("source is nil")
	}
	equal(t, source.Dialect, DialectMySQL)
	equal(t, *source.Version, "8.0.46")
	equal(t, *source.Producer, "mysqldump")
	equal(t, *source.Charset, "utf8mb4")
	equal(t, source.Databases, []string{})

	equal(t, len(report.Tables), 2)

	customers := report.Tables[0]
	equal(t, customers.SourceName, "customers")
	equal(t, customers.Name, "customers")
	equal(t, customers.Schema, (*string)(nil))
	equal(t, customers.Rows, int64(8))
	equal(t, len(customers.Columns), 7)
	equal(t, customers.Columns[5].Name, "signed_up")
	equal(t, customers.Columns[5].Kind, ColumnKind("date"))
	equal(t, customers.Columns[5].Fixes, []int{2, 3})
	equal(t, customers.Fixes[2].Code, FixCode("zero_date_default"))
	equal(t, customers.Fixes[2].Args["value"], "0000-00-00")
	equal(t, customers.Problems[0].Code, ErrorCode("invalid_date_null"))
	equal(t, customers.Problems[0].Severity, Severity("warning"))
	equal(t, *customers.Problems[0].Column, "signed_up")

	orders := report.Tables[1]
	equal(t, orders.Name, "orders")
	equal(t, orders.Rows, int64(10))
	equal(t, orders.Columns[3].SQLType, "numeric(10,2)")
	equal(t, *orders.Columns[3].Precision, 10)
	equal(t, *orders.Columns[3].Scale, 2)
	equal(t, len(orders.Fixes), 5)
	equal(t, orders.Fixes[4].Code, FixCode("on_update_trigger"))
	equal(t, *orders.Problems[0].Column, "shipped_at")

	equal(t, report.NotConverted, []NotConverted{
		{Kind: ObjectView, Name: "active_customers"},
		{Kind: ObjectTrigger, Name: "orders_bi"},
	})
}

func TestAReportWithoutADumpHasNoSourceOrTables(t *testing.T) {
	report := decodeReport(t, []byte(`{"version":1,"input":{"format":"csv","encoding":"utf-8","bytes":10},
		"dialect":"postgresql","table":"people","columns":[],"rows":{"read":1,"converted":1,"rejected":0},
		"fixes":[],"problems":[],"timings":{"read_ms":0,"convert_ms":0,"total_ms":0}}`))

	equal(t, report.Source, (*SourceReport)(nil))
	equal(t, report.Tables, []TableReport(nil))
	equal(t, report.NotConverted, []NotConverted(nil))

	written, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}

	var fields map[string]any
	if err := json.Unmarshal(written, &fields); err != nil {
		t.Fatal(err)
	}

	for _, field := range []string{"source", "tables", "not_converted"} {
		if _, ok := fields[field]; ok {
			t.Errorf("%s is written for a report without a dump", field)
		}
	}
}

func TestUnknownKindsAndCodesStillDecode(t *testing.T) {
	report := decodeReport(t, []byte(`{"source":{"dialect":"cockroachdb","databases":["shop"]},
		"tables":[{"source_name":"t","name":"t","rows":1,"columns":[{"source_name":"c","name":"c",
		"sql_type":"vector","kind":"vector","nullable":true,"fixes":[1]}],
		"fixes":[{"number":1,"code":"a_fix_added_later","args":{}}],
		"problems":[{"code":"a_problem_added_later","severity":"notice","args":{}}]}],
		"not_converted":[{"kind":"sequence","name":"ids"}]}`))

	equal(t, report.Source.Dialect, Dialect("cockroachdb"))
	equal(t, report.Source.Version, (*string)(nil))
	equal(t, report.Source.Databases, []string{"shop"})
	equal(t, report.Tables[0].Columns[0].Kind, ColumnKind("vector"))
	equal(t, report.Tables[0].Fixes[0].Code, FixCode("a_fix_added_later"))
	equal(t, report.Tables[0].Problems[0].Code, ErrorCode("a_problem_added_later"))
	equal(t, report.NotConverted[0].Kind, ObjectKind("sequence"))
}
