// Package intosql is the Go client for the IntoSQL API, which converts spreadsheets (XLSX, XLS,
// ODS), CSV and JSON into SQL for PostgreSQL, MySQL, MariaDB, SQLite, SQL Server or Oracle.
//
// The API is part of IntoSQL Pro. Make an API key in Settings, API keys, then:
//
//	client, err := intosql.NewClient(os.Getenv("INTOSQL_API_KEY"))
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	conversion, err := client.ConvertFile(ctx, "ventas.xlsx", intosql.ConvertOptions{
//		Spec: intosql.NewSpec(intosql.SpecOptions{Dialect: intosql.DialectPostgreSQL, Table: "ventas"}),
//	})
//
// Every endpoint is documented at https://intosql.com/docs/sdks/go.
package intosql
