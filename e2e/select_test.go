package e2e

import (
	"bytes"
	"encoding/csv"
	jsonpkg "encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"
	"gotest.tools/v3/icmd"
)

func getFile(n int, inputForm, outputForm, structure string) (string, string) {
	type row struct {
		Line string `json:"line"`
		ID   string `json:"id"`
		Data string `json:"data"`
	}
	var data []row

	for i := 0; i < n; i++ {
		data = append(data, row{
			Line: fmt.Sprintf("%d", i),
			ID:   fmt.Sprintf("id%d", i),
			Data: fmt.Sprintf("some event %d", i),
		})
	}
	var input bytes.Buffer
	var expected bytes.Buffer
	switch inputForm {
	case "json":
		encoder := jsonpkg.NewEncoder(&input)
		switch structure {
		case "document":
			rows := make(map[string]row)
			for i, v := range data {
				rows[fmt.Sprintf("obj%d", i)] = v
			}
			err := encoder.Encode(rows)
			if err != nil {
				panic(err)
			}
			return input.String(), input.String()
		default:
			for _, d := range data {
				err := encoder.Encode(d)
				if err != nil {
					panic(err)
				}
			}
			switch outputForm {
			case "json":
				return input.String(), input.String()
			case "csv":
				writer := csv.NewWriter(&expected)
				for _, d := range data {
					err := writer.Write([]string{d.Line, d.ID, d.Data})
					if err != nil {
						panic(err)
					}
				}
				writer.Flush()

				return input.String(), expected.String()
			}
		}

		// edge case

	case "csv":
		writer := csv.NewWriter(&input)
		writer.Comma = []rune(structure)[0]
		writer.Write([]string{"line", "id", "data"})
		for _, d := range data {
			writer.Write([]string{d.Line, d.ID, d.Data})
		}
		writer.Flush()
		switch outputForm {
		case "json":
			encoder := jsonpkg.NewEncoder(&expected)
			encoder.Encode(map[string]string{
				"_1": "line",
				"_2": "id",
				"_3": "data",
			})
			for _, d := range data {
				encoder.Encode(map[string]string{
					"_1": d.Line,
					"_2": d.ID,
					"_3": d.Data,
				})
			}
			return input.String(), expected.String()
		case "csv":
			writer := csv.NewWriter(&expected)
			writer.Write([]string{"line", "id", "data"})
			for _, d := range data {
				writer.Write([]string{d.Line, d.ID, d.Data})
			}
			writer.Flush()

			return input.String(), expected.String()
		}
	}
	panic("unreachable")
}

/*
test cases:
json:

	default input structure and output
	default input structure and output as csv
	document input structure and default output
	document input structure and output as csv

csv:

	default input structure and output
	default input structure and output as csv
	tab input structure and default output
	tab input structure and output as csv

parquet:

	// TODO: discuss about testing
	default input structure and output
	default input structure and output as csv
*/
func TestSelectCommand(t *testing.T) {
	t.Parallel()
	// credentials are same for all test cases
	region := "us-east-1"
	accessKeyID := "minioadmin"
	secretKey := "minioadmin"
	host := os.Getenv("MINIO_HOST")
	port := os.Getenv("MINIO_PORT")
	address := fmt.Sprintf("http://%s:%s", host, port)
	// The query is default for all cases, we want to assert the output
	// is as expected after a query.
	query := "SELECT * FROM s3object s LIMIT 6"
	testcases := []struct {
		name      string
		cmd       []string
		in        string
		structure string
		out       string
		expected  map[int]compareFunc
		src       string
	}{
		// json
		{
			name: "json-lines select with default input structure and output",
			cmd: []string{
				"select",
				"json",
				"--query",
				query,
			},
			in:        "json",
			structure: "lines",
			out:       "json",
		},
		{
			name: "json-lines select with default input structure and csv output",
			cmd: []string{
				"select",
				"json",
				"--output-format",
				"csv",
				"--query",
				query,
			},
			in:        "json",
			structure: "lines",
			out:       "csv",
		},
		{
			name: "json-lines select with document input structure and output",
			cmd: []string{
				"select",
				"json",
				"--structure",
				"document",
				"--query",
				query,
			},
			in:        "json",
			structure: "document",
			out:       "json",
		},
		/* {
			name: "json-lines select with document input structure and csv output",
			cmd: []string{
				"select",
				"json",
				"--structure",
				"document",
				"--output-format",
				"csv",
				"--query",
				query,
			},
			in:        "json",
			structure: "document",
			out:       "csv",
		}, */ // This case is not supported by AWS itself.
		// csv
		{
			name: "csv select with default delimiter and output",
			cmd: []string{
				"select",
				"csv",
				"--query",
				query,
			},
			in:        "csv",
			structure: ",",
			out:       "json",
		},
		{
			name: "csv select with default delimiter and csv output",
			cmd: []string{
				"select",
				"csv",
				"--output-format",
				"csv",
				"--query",
				query,
			},
			in:        "csv",
			structure: ",",
			out:       "csv",
		},
		{
			name: "csv select with custom delimiter and default output",
			cmd: []string{
				"select",
				"csv",
				"--delimiter",
				"\t",
				"--query",
				query,
			},
			in:        "csv",
			structure: "\t",
			out:       "json",
		},
		{
			name: "csv select with custom delimiter and csv output",
			cmd: []string{
				"select",
				"csv",
				"--delimiter",
				"\t",
				"--output-format",
				"csv",
				"--query",
				query,
			},
			in:        "csv",
			structure: "\t",
			out:       "csv",
		},
	}
	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			contents, expected := getFile(5, tc.in, tc.out, tc.structure)
			filename := fmt.Sprintf("file.%s", tc.in)
			bucket := s3BucketFromTestName(t)
			src := fmt.Sprintf("s3://%s/%s", bucket, filename)
			tc.cmd = append(tc.cmd, src)

			s3client, s5cmd := setup(t, withEndpointURL(address), withRegion(region), withAccessKeyID(accessKeyID), withSecretKey(secretKey))
			createBucket(t, s3client, bucket)
			putFile(t, s3client, bucket, filename, contents)
			cmd := s5cmd(tc.cmd...)

			result := icmd.RunCmd(cmd, withEnv("AWS_ACCESS_KEY_ID", accessKeyID), withEnv("AWS_SECRET_ACCESS_KEY", secretKey))

			if diff := cmp.Diff(expected, result.Stdout()); diff != "" {
				t.Errorf("select command mismatch (-want +got):\n%s", diff)
			}

		})
	}
}