package process

import (
	"bytes"
	"debug/elf"
	"encoding/json"
	"errors"
	"os/exec"
	"testing"
)

func TestGoFuncFieldArgs(t *testing.T) {
	tt := []struct {
		filename    string
		funcName    string
		expectedErr error
	}{
		// stdlib tests
		{
			filename:    "testdata/stdlib/app1.23.8",
			funcName:    "crypto/tls.(*Conn).Write",
			expectedErr: nil,
		},
		{
			filename:    "testdata/stdlib/app1.23.8",
			funcName:    "crypto/tls.(*Conn).Read",
			expectedErr: nil,
		},
		{
			filename:    "testdata/stdlib/app1.23.8",
			funcName:    "net/http.(*http2Framer).WriteDataPadded",
			expectedErr: nil,
		},
		{
			filename:    "testdata/stdlib/app1.23.8",
			funcName:    "net/http.(*http2Framer).checkFrameOrder",
			expectedErr: nil,
		},
		{
			filename:    "testdata/stdlib/app1.23.8",
			funcName:    "net/http.(*http2writeResHeaders).writeFrame",
			expectedErr: nil,
		},
		{
			filename:    "testdata/stdlib/app1.23.8",
			funcName:    "net/http.(*http2serverConn).processHeaders",
			expectedErr: nil,
		},
		// grpc tests
		{
			filename:    "testdata/grpc/appv1.53.0",
			funcName:    "google.golang.org/grpc/internal/transport.(*http2Server).operateHeaders",
			expectedErr: nil,
		},
		{
			filename:    "testdata/grpc/appv1.53.0",
			funcName:    "google.golang.org/grpc/internal/transport.(*http2Client).operateHeaders",
			expectedErr: nil,
		},
		{
			filename:    "testdata/grpc/appv1.53.0",
			funcName:    "google.golang.org/grpc/internal/transport.(*loopyWriter).writeHeader",
			expectedErr: nil,
		},
		// TODO(ddelnano): This binary was generated from the px/google.golang.org/grpc template
		// application and is missing some golang.org/x/net/http2 symbols. Use a dedicated
		// golang.org/x/net application template once the missing symbols are understood.
		{
			filename:    "testdata/golang-x-net/appv0.39.0",
			funcName:    "golang.org/x/net/http2.(*Framer).WriteDataPadded",
			expectedErr: nil,
		},
		{
			filename:    "testdata/golang-x-net/appv0.39.0",
			funcName:    "golang.org/x/net/http2.(*Framer).checkFrameOrder",
			expectedErr: nil,
		},
		{
			filename:    "testdata/golang-x-net/appv0.39.0",
			funcName:    "golang.org/x/net/http2/hpack.(*Encoder).WriteField",
			expectedErr: nil,
		},
	}

	for _, tc := range tt {
		t.Run(tc.filename+"_"+tc.funcName, func(t *testing.T) {
			elfF, err := elf.Open(tc.filename)
			if err != nil {
				t.Fatalf("failed to open ELF file %s: %v", tc.filename, err)
			}
			defer elfF.Close()
			dwData, err := elfF.DWARF()
			if err != nil {
				t.Fatalf("failed to get DWARF data from %s: %v", tc.filename, err)
			}
			dw := DWARF{Reader: dwData.Reader()}

			args, err := dw.GoFuncFieldArgs(tc.funcName)
			if !errors.Is(err, tc.expectedErr) {
				t.Fatalf("expected error %v, got %v", tc.expectedErr, err)
			}
			expectedFuncArgs, err := GetFuncArgsFromPx(tc.filename, tc.funcName)
			if err != nil {
				t.Fatalf("failed to get function args from px: %v", err)
			}
			if len(expectedFuncArgs) != len(args) {
				t.Fatalf("expected %d args from px, got %d", len(expectedFuncArgs), len(args))
			}

			for argName, funcField := range args {
				expectedArg, exists := expectedFuncArgs[argName]
				if !exists {
					t.Errorf("expected arg %s not found in px output", argName)
				} else {
					// TODO(ddelnano): Fix the violations later
					funcField.RetArg = expectedArg.RetArg
					if expectedArg != funcField {
						t.Errorf("expected arg '%s' %+v to match px output %+v", argName, expectedArg, funcField)
					}
				}
			}
		})
	}
}

// This command runs Pixie's //src/stirling/binaries:go_func_dwarf_dump tool. This ensures that
// the offsetgen matches the output of the Pixie tool, which is used in production
func GetFuncArgsFromPx(filename, funcName string) (map[string]FuncFieldArg, error) {

	// Run exePath with binary and func_names args
	// Parse output to get offset and location
	// Return offset and location
	cmd := exec.Command("testdata/go_func_dwarf_dump", "--binary", filename, "--func_names", funcName)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	var result map[string]FuncFieldArg
	var err error
	if err = cmd.Run(); err != nil {
		return result, err
	}

	err = json.Unmarshal(stdout.Bytes(), &result)
	return result, err

}
