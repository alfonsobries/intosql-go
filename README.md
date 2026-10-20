# IntoSQL for Go

Convert spreadsheets (XLSX, XLS, ODS), CSV and JSON into SQL for PostgreSQL, MySQL, MariaDB, SQLite, SQL Server or Oracle with the [IntoSQL API](https://intosql.com/developers). Column types are inferred and dirty values fixed, so the script loads on the first try.

```sh
go get github.com/alfonsobries/intosql-go
```

Go 1.22 or newer, standard library only. The API is part of IntoSQL Pro; make a key in Settings, API keys. Full guide: [intosql.com/docs/sdks/go](https://intosql.com/docs/sdks/go).

## Convert a file

```go
import "github.com/alfonsobries/intosql-go"

client, err := intosql.NewClient(os.Getenv("INTOSQL_API_KEY"))
if err != nil {
	log.Fatal(err)
}

conversion, err := client.ConvertFile(ctx, "ventas.xlsx", intosql.ConvertOptions{
	Spec: intosql.NewSpec(intosql.SpecOptions{Dialect: intosql.DialectPostgreSQL, Table: "ventas"}),
})
if err != nil {
	log.Fatal(err)
}

fmt.Println(conversion.Report.Rows.Converted, conversion.Output.URL)
```

The file streams to storage as it is; the output URL downloads the script for ten minutes. Read the conversion again (`client.Conversions.Retrieve(ctx, id)`) for a fresh one. `client.Convert(ctx, reader, size, options)` takes any `io.Reader` with its size.

A MySQL, MariaDB, PostgreSQL or SQL Server dump (`.sql`, `.sql.gz`) converts to any of the six databases like any file. The report then lists each table in `Tables`, with its own columns, fixes and problems, and the views, triggers and routines it did not carry in `NotConverted`.

```go
for _, table := range conversion.Report.Tables {
	fmt.Println(table.Name, table.Rows)
}
```

## Convert a file by URL

```go
conversion, err := client.ConvertURL(ctx, "https://example.com/exports/ventas.csv", intosql.ConvertURLOptions{
	Spec: intosql.NewSpec(intosql.SpecOptions{Dialect: intosql.DialectMySQL}),
})
```

## Check a webhook

Save a webhook address in Settings, API keys, and IntoSQL calls it when a conversion is done, failed or canceled.

```go
body, _ := io.ReadAll(request.Body)

// Returns intosql.ErrInvalidSignature when the signature doesn't match.
event, err := intosql.ParseWebhook(body, request.Header.Get("IntoSQL-Signature"), os.Getenv("INTOSQL_WEBHOOK_SECRET"))
```

## Errors and retries

Failed requests return `*intosql.Error` with the HTTP `Status`, a `Code` that doesn't change (`needs_pro`, `over_quota`, `too_big`, `invalid_spec`, `invalid_url` and more) and the `RequestID`. A conversion that ends as failed returns `*intosql.ConversionFailedError` with its `Code` and the engine's `Problems`. Use `errors.As` to tell them apart. Rate limits, a full account, server errors and lost connections are retried twice with the same idempotency key (`intosql.WithMaxRetries` changes it); `already_started` never is.

Every endpoint: [intosql.com/developers](https://intosql.com/developers).

## License

MIT
