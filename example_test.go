package intosql_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/alfonsobries/intosql-go"
)

// These examples compile with the package, so the documentation's code stays true. They have no
// output and are not run.

func ExampleClient_ConvertFile() {
	ctx := context.Background()

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
}

func ExampleClient_Convert() {
	ctx := context.Background()
	client, _ := intosql.NewClient(os.Getenv("INTOSQL_API_KEY"))

	response, err := http.Get("https://example.com/exports/pedidos.json")
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	conversion, err := client.Convert(ctx, response.Body, response.ContentLength, intosql.ConvertOptions{
		Name: "pedidos.json",
		Spec: intosql.NewSpec(intosql.SpecOptions{Dialect: intosql.DialectSQLite}),
		Wait: intosql.WaitOptions{Timeout: 10 * time.Minute},
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(conversion.ID)
}

func ExampleClient_ConvertURL() {
	ctx := context.Background()
	client, _ := intosql.NewClient(os.Getenv("INTOSQL_API_KEY"))

	conversion, err := client.ConvertURL(ctx, "https://example.com/exports/ventas.csv", intosql.ConvertURLOptions{
		Spec: intosql.NewSpec(intosql.SpecOptions{Dialect: intosql.DialectMySQL}),
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(conversion.Output.URL)
}

func ExampleConversions_List() {
	ctx := context.Background()
	client, _ := intosql.NewClient(os.Getenv("INTOSQL_API_KEY"))

	conversions := client.Conversions.List(ctx, intosql.ListParams{Status: intosql.StatusFailed})
	for conversions.Next() {
		conversion := conversions.Current()
		fmt.Println(conversion.ID, conversion.Error.Code, conversion.Name)
	}

	if err := conversions.Err(); err != nil {
		log.Fatal(err)
	}
}

func ExampleConversions_Create() {
	ctx := context.Background()
	client, _ := intosql.NewClient(os.Getenv("INTOSQL_API_KEY"))

	file, err := os.Open("ventas.xlsx")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	info, _ := file.Stat()

	opened, err := client.Conversions.Create(ctx, intosql.CreateParams{
		Name:  "ventas.xlsx",
		Bytes: info.Size(),
		Spec:  intosql.NewSpec(intosql.SpecOptions{Dialect: intosql.DialectMariaDB}),
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := client.Conversions.Upload(ctx, opened, file, info.Size()); err != nil {
		log.Fatal(err)
	}

	if _, err := client.Conversions.Start(ctx, opened.ID); err != nil {
		log.Fatal(err)
	}

	// Later, or from a webhook handler:
	conversion, err := client.Conversions.Retrieve(ctx, opened.ID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(conversion.Status)
}

func ExampleConversions_CreateCase() {
	ctx := context.Background()
	client, _ := intosql.NewClient(os.Getenv("INTOSQL_API_KEY"))

	_, err := client.Conversions.CreateCase(ctx, "01k7r3j9qf8m2v6x4t0c5n1h8d", intosql.CaseParams{
		Consent: true,
		Share:   intosql.ShareFile,
		Note:    "Exported from our ERP; the dates come out wrong.",
	})
	if err != nil {
		log.Fatal(err)
	}
}

func ExampleParseWebhook() {
	http.HandleFunc("/webhooks/intosql", func(writer http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)

			return
		}

		event, err := intosql.ParseWebhook(body, request.Header.Get("IntoSQL-Signature"), os.Getenv("INTOSQL_WEBHOOK_SECRET"))
		if err != nil {
			writer.WriteHeader(http.StatusBadRequest)

			return
		}

		if event.Type == intosql.EventConversionDone {
			// event.Conversion is the conversion; event.ID is the same on every retry.
			fmt.Println(event.Conversion.Output.URL)
		}

		writer.WriteHeader(http.StatusNoContent)
	})
}

func ExampleError() {
	ctx := context.Background()
	client, _ := intosql.NewClient(os.Getenv("INTOSQL_API_KEY"))

	_, err := client.ConvertFile(ctx, "ventas.xlsx", intosql.ConvertOptions{
		Spec: intosql.NewSpec(intosql.SpecOptions{Dialect: intosql.DialectOracle}),
	})

	var failed *intosql.ConversionFailedError
	var apiError *intosql.Error

	switch {
	case errors.As(err, &failed):
		log.Printf("Conversion %s failed (%s): %s", failed.ID, failed.Code, failed.Message)
	case errors.As(err, &apiError) && apiError.Code == "over_quota":
		log.Print("This month's conversions are used up.")
	case errors.As(err, &apiError):
		log.Print(apiError.Status, apiError.Code, apiError.Message, apiError.Fields)
	case err != nil:
		log.Fatal(err)
	}
}
