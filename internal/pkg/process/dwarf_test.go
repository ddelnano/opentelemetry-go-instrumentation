package process

import (
	"debug/elf"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/auto/internal/pkg/funcfield"
)

func TestDWARFFunctionOffsets(t *testing.T) {

	// Path to the compiled test binary containing the function.
	binaryPath := "testdata/client"

	// Open the ELF binary.
	file, err := os.Open(binaryPath)
	assert.NoError(t, err, "Failed to open test binary")
	defer file.Close()

	// Parse the ELF binary.
	elfFile, err := elf.NewFile(file)
	assert.NoError(t, err, "Failed to parse ELF file")

	// Extract DWARF data from the ELF binary.
	dwarfData, err := elfFile.DWARF()
	dwarfParser := DWARF{Data: dwarfData, Reader: dwarfData.Reader()} // Define the function ID for `crypto/tls.(*Conn).Read`.

	args, err := dwarfParser.GoFunctionArguments(funcfield.ID{
		Name:    "crypto/tls.(*Conn).Read",
		Args:    []string{"c", "b"},
		Retvals: []int{0, 1},
	})

	if err != nil {
		t.Fatalf("Failed to get function arguments: %v", err)
	}
	fmt.Printf("Function arguments: %+v\n", args)

	if len(args) == 0 {
		t.Fatalf("Function arguments not found")
	}

	if _, ok := args["c"]; !ok {
		t.Fatalf("Argument 'c' not found")
	}

	if _, ok := args["b"]; !ok {
		t.Fatalf("Argument 'b' not found")
	}

	expectedArgC := ArgInfo{
		TypeInfo: TypeInfo{
			VarType:  Pointer,
			TypeName: "*crypto/tls.Conn",
			DeclType: "*crypto/tls.Conn",
		},
		Location: VarLocation{
			LocType: Register,
			Offset:  0,
			Regs: []RegisterName{
				kRAX,
			},
		},
		RetArg: false,
	}

	argC := args["c"]
	assert.Equal(t, expectedArgC, argC, "Argument 'c' does not match expected value")

	expectedArgB := ArgInfo{
		TypeInfo: TypeInfo{
			VarType:  Struct,
			TypeName: "[]uint8",
			DeclType: "[]uint8",
		},
		Location: VarLocation{
			LocType: Register,
			Offset:  8,
			Regs: []RegisterName{
				kRBX,
				kRCX,
				kRDI,
			},
		},
		RetArg: false,
	}
	argB := args["b"]
	assert.Equal(t, expectedArgB, argB, "Argument 'b' does not match expected value")
}
