// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Offsetgen is a utility to generate a static file containing offsets for Go
// struct fields.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime"

	"go.opentelemetry.io/auto/internal/pkg/funcfield"
	"go.opentelemetry.io/auto/internal/pkg/structfield"
	"go.opentelemetry.io/auto/internal/tools/inspect"
)

const (
	defaultOutputFile = "offset_results.json"

	minGoVersion = "1.19"
)

var (
	// outputFile is the output file path flag value.
	outputFile string
	// cacheFile is the offset cache file path flag value.
	cacheFile string
	// numCPU is the number of CPUs to use flag value.
	numCPU int
	// verbosity is the log verbosity level flag value.
	verbosity int

	logger *slog.Logger
)

func init() {
	flag.StringVar(&outputFile, "output", defaultOutputFile, "output file")
	flag.StringVar(&cacheFile, "cache", "", "offset cache")
	flag.IntVar(&numCPU, "workers", runtime.NumCPU(), "max number of Goroutine workers")
	flag.IntVar(&verbosity, "v", 0, "log verbosity")

	flag.Parse()

	logger = slog.Default()
}

func manifests() ([]inspect.Manifest, error) {
	goVers, err := GoVersions(">= " + minGoVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get Go versions: %w", err)
	}

	grpcVers, err := PkgVersions("google.golang.org/grpc")
	if err != nil {
		return nil, fmt.Errorf("failed to get \"google.golang.org/grpc\" versions: %w", err)
	}

	xNetVers, err := PkgVersions("golang.org/x/net")
	if err != nil {
		return nil, fmt.Errorf("failed to get \"golang.org/x/net\" versions: %w", err)
	}

	ren := func(src string) inspect.Renderer {
		return inspect.NewRenderer(logger, src, inspect.DefaultFS)
	}

	return []inspect.Manifest{
		{
			Application: inspect.Application{
				Renderer:  ren("templates/crypto/tls/*.tmpl"),
				GoVerions: goVers,
			},
			Funcs: []funcfield.ID{
				funcfield.ID{
					ModPath: "std",
					PkgPath: "crypto/tls",
					Func:    "(*Conn).Read",
					Arg:     "c",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "crypto/tls",
					Func:    "(*Conn).Read",
					Arg:     "b",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "crypto/tls",
					Func:    "(*Conn).Read",
					Arg:     "~r0",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "crypto/tls",
					Func:    "(*Conn).Read",
					Arg:     "~r1",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "crypto/tls",
					Func:    "(*Conn).Write",
					Arg:     "c",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "crypto/tls",
					Func:    "(*Conn).Write",
					Arg:     "b",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "crypto/tls",
					Func:    "(*Conn).Write",
					Arg:     "~r0",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "crypto/tls",
					Func:    "(*Conn).Write",
					Arg:     "~r1",
				},
			},
		},
		{
			Application: inspect.Application{
				Renderer:  ren("templates/runtime/*.tmpl"),
				GoVerions: goVers,
			},
			StructFields: []structfield.ID{
				structfield.NewID("std", "runtime", "g", "goid"),
				structfield.NewID("std", "runtime", "hmap", "buckets"),
			},
		},
		{
			Application: inspect.Application{
				Renderer:  ren("templates/net/http/*.tmpl"),
				GoVerions: goVers,
			},
			StructFields: []structfield.ID{

				// Added for Pixie's TLS tracing
				structfield.NewID("std", "internal/poll", "FD", "Sysfd"),
				structfield.NewID("std", "crypto/tls", "Conn", "conn"),
				structfield.NewID("std", "net/http", "http2serverConn", "conn"),
				structfield.NewID("std", "net/http", "http2serverConn", "hpackEncoder"),
				structfield.NewID("std", "net/http", "http2HeadersFrame", "http2FrameHeader"),
				structfield.NewID("std", "net/http", "http2FrameHeader", "Type"),
				structfield.NewID("std", "net/http", "http2FrameHeader", "Flags"),
				structfield.NewID("std", "net/http", "http2FrameHeader", "StreamID"),
				structfield.NewID("std", "net/http", "http2DataFrame", "data"),
				structfield.NewID("std", "net/http", "http2writeResHeaders", "streamID"),
				structfield.NewID("std", "net/http", "http2writeResHeaders", "endStream"),
				structfield.NewID("std", "net/http", "http2MetaHeadersFrame", "http2HeadersFrame"),
				structfield.NewID("std", "net/http", "http2MetaHeadersFrame", "Fields"),
				structfield.NewID("std", "net/http", "http2Framer", "w"),
				structfield.NewID("std", "net/http", "http2bufferedWriter", "w"),
			},
		},
		{
			Application: inspect.Application{
				Renderer:  ren("templates/net/http/*.tmpl"),
				GoVerions: goVers,
			},
			Funcs: []funcfield.ID{
				// net/http.(*http2Framer).WriteDataPadded
				funcfield.ID{
					ModPath: "std",
					PkgPath: "net/http",
					Func:    "(*http2Framer).WriteDataPadded",
					Arg:     "f",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "net/http",
					Func:    "(*http2Framer).WriteDataPadded",
					Arg:     "streamID",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "net/http",
					Func:    "(*http2Framer).WriteDataPadded",
					Arg:     "endStream",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "net/http",
					Func:    "(*http2Framer).WriteDataPadded",
					Arg:     "data",
				},
				// net/http.(*http2Framer).checkFrameOrder
				funcfield.ID{
					ModPath: "std",
					PkgPath: "net/http",
					Func:    "(*http2Framer).checkFrameOrder",
					Arg:     "fr",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "net/http",
					Func:    "(*http2Framer).checkFrameOrder",
					Arg:     "f",
				},
				// net/http.(*http2writeResHeaders).writeFrame
				funcfield.ID{
					ModPath: "std",
					PkgPath: "net/http",
					Func:    "(*http2writeResHeaders).writeFrame",
					Arg:     "w",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "net/http",
					Func:    "(*http2writeResHeaders).writeFrame",
					Arg:     "ctx",
				},
				// net/http.(*http2serverConn).processHeaders
				funcfield.ID{
					ModPath: "std",
					PkgPath: "net/http",
					Func:    "(*http2serverConn).processHeaders",
					Arg:     "sc",
				},
				funcfield.ID{
					ModPath: "std",
					PkgPath: "net/http",
					Func:    "(*http2serverConn).processHeaders",
					Arg:     "f",
				},
			},
		},
		{
			Application: inspect.Application{
				Renderer: ren("templates/google.golang.org/grpc/*.tmpl"),
				Versions: grpcVers,
			},
			StructFields: []structfield.ID{

				// Added for Pixie's GRPC tracing
				structfield.NewID("google.golang.org/grpc", "google.golang.org/grpc/internal/transport", "http2Server", "conn"),
				structfield.NewID("google.golang.org/grpc", "google.golang.org/grpc/internal/transport", "http2Client", "conn"),
				structfield.NewID("google.golang.org/grpc", "google.golang.org/grpc/internal/transport", "loopyWriter", "framer"),

				// Added for Pixie's TLS tracing
				structfield.NewID("google.golang.org/grpc", "google.golang.org/grpc/internal/transport", "bufWriter", "conn"),

				// TODO(ddelnano): This field is optional and when added to the offsetgen I'm not able to see the offsets be found.
				// This may require adding this field to the templated application for it to work.
				structfield.NewID("google.golang.org", "google.golang.org/grpc/credentials/internal", "syscallConn", "conn"),
			},
		},
		{
			Application: inspect.Application{
				Renderer: ren("templates/px/google.golang.org/grpc/*.tmpl"),
				Versions: grpcVers,
			},
			Funcs: []funcfield.ID{
				// google.golang.org/grpc/internal/transport.(*http2Server).operateHeaders
				funcfield.ID{
					ModPath: "google.golang.org/grpc",
					PkgPath: "google.golang.org/grpc/internal/transport",
					Func:    "(*http2Server).operateHeaders",
					Arg:     "t",
				},
				funcfield.ID{
					ModPath: "google.golang.org/grpc",
					PkgPath: "google.golang.org/grpc/internal/transport",
					Func:    "(*http2Server).operateHeaders",
					Arg:     "frame",
				},
				// google.golang.org/grpc/internal/transport.(*http2Client).operateHeaders
				funcfield.ID{
					ModPath: "google.golang.org/grpc",
					PkgPath: "google.golang.org/grpc/internal/transport",
					Func:    "(*http2Client).operateHeaders",
					Arg:     "t",
				},
				funcfield.ID{
					ModPath: "google.golang.org/grpc",
					PkgPath: "google.golang.org/grpc/internal/transport",
					Func:    "(*http2Client).operateHeaders",
					Arg:     "frame",
				},
				// google.golang.org/grpc/internal/transport.(*loopyWriter).writeHeader
				funcfield.ID{
					ModPath: "google.golang.org/grpc",
					PkgPath: "google.golang.org/grpc/internal/transport",
					Func:    "(*loopyWriter).writeHeader",
					Arg:     "l",
				},
				funcfield.ID{
					ModPath: "google.golang.org/grpc",
					PkgPath: "google.golang.org/grpc/internal/transport",
					Func:    "(*loopyWriter).writeHeader",
					Arg:     "streamID",
				},
				funcfield.ID{
					ModPath: "google.golang.org/grpc",
					PkgPath: "google.golang.org/grpc/internal/transport",
					Func:    "(*loopyWriter).writeHeader",
					Arg:     "endStream",
				},
				funcfield.ID{
					ModPath: "google.golang.org/grpc",
					PkgPath: "google.golang.org/grpc/internal/transport",
					Func:    "(*loopyWriter).writeHeader",
					Arg:     "hf",
				},
			},
		},
		{
			Application: inspect.Application{
				Renderer: ren("templates/golang.org/x/net/*.tmpl"),
				Versions: xNetVers,
			},
			StructFields: []structfield.ID{
				// Upstream but needed by Pixie's GRPC tracing
				structfield.NewID("golang.org/x/net", "golang.org/x/net/http2", "MetaHeadersFrame", "Fields"),
				structfield.NewID("golang.org/x/net", "golang.org/x/net/http2", "FrameHeader", "StreamID"),

				// Added for Pixie's GRPC tracing
				structfield.NewID("golang.org/x/net", "golang.org/x/net/http2/hpack", "HeaderField", "Name"),
				structfield.NewID("golang.org/x/net", "golang.org/x/net/http2/hpack", "HeaderField", "Value"),
				structfield.NewID("golang.org/x/net", "golang.org/x/net/http2/hpack", "HeaderField", "Value"),
				structfield.NewID("golang.org/x/net", "golang.org/x/net/http2", "MetaHeadersFrame", "HeadersFrame"),
				structfield.NewID("golang.org/x/net", "golang.org/x/net/http2", "HeadersFrame", "FrameHeader"),
				structfield.NewID("golang.org/x/net", "golang.org/x/net/http2", "FrameHeader", "Type"),
				structfield.NewID("golang.org/x/net", "golang.org/x/net/http2", "FrameHeader", "Flags"),
				structfield.NewID("golang.org/x/net", "golang.org/x/net/http2", "DataFrame", "data"),
				structfield.NewID("golang.org/x/net", "golang.org/x/net/http2", "Framer", "w"),
			},
		},
		{
			Application: inspect.Application{
				Renderer: ren("templates/golang.org/x/net/*.tmpl"),
				Versions: xNetVers,
			},
			Funcs: []funcfield.ID{
				// golang.org/x/net/http2.(*Framer).WriteDataPadded
				funcfield.ID{
					ModPath: "golang.org/x/net",
					PkgPath: "golang.org/x/net/http2",
					Func:    "(*Framer).WriteDataPadded",
					Arg:     "f",
				},
				funcfield.ID{
					ModPath: "golang.org/x/net",
					PkgPath: "golang.org/x/net/http2",
					Func:    "(*Framer).WriteDataPadded",
					Arg:     "streamID",
				},
				funcfield.ID{
					ModPath: "golang.org/x/net",
					PkgPath: "golang.org/x/net/http2",
					Func:    "(*Framer).WriteDataPadded",
					Arg:     "endStream",
				},
				funcfield.ID{
					ModPath: "golang.org/x/net",
					PkgPath: "golang.org/x/net/http2",
					Func:    "(*Framer).WriteDataPadded",
					Arg:     "data",
				},
				// golang.org/x/net/http2.(*Framer).checkFrameOrder
				funcfield.ID{
					ModPath: "golang.org/x/net",
					PkgPath: "golang.org/x/net/http2",
					Func:    "(*Framer).checkFrameOrder",
					Arg:     "fr",
				},
				funcfield.ID{
					ModPath: "golang.org/x/net",
					PkgPath: "golang.org/x/net/http2",
					Func:    "(*Framer).checkFrameOrder",
					Arg:     "f",
				},
				// golang.org/x/net/http2/hpack.(*Encoder).WriteField
				funcfield.ID{
					ModPath: "golang.org/x/net",
					PkgPath: "golang.org/x/net/http2/hpack",
					Func:    "(*Encoder).WriteField",
					Arg:     "e",
				},
				funcfield.ID{
					ModPath: "golang.org/x/net",
					PkgPath: "golang.org/x/net/http2/hpack",
					Func:    "(*Encoder).WriteField",
					Arg:     "f",
				},
			},
		},
	}, nil
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	m, err := manifests()
	if err != nil {
		logger.Error("failed to load manifests", "error", err)
		return err
	}

	var cache *inspect.Cache
	if cacheFile != "" {
		cache, err = inspect.NewCache(logger, cacheFile)
		if err != nil {
			logger.Error("failed to load cache", "error", err, "path", cacheFile)
			// Use an empty cache.
		}
	}

	i, err := inspect.New(logger, cache, m...)
	if err != nil {
		logger.Error("failed to setup inspector", "error", err)
		return err
	}
	i.NWorkers = numCPU

	// Trap Ctrl+C and call cancel on the context.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	to, err := i.Do(ctx)
	if err != nil {
		logger.Error("failed get offsets", "error", err)
		return err
	}

	if to == nil {
		logger.Info("no offsets found")
		return nil
	}

	logger.Info("writing offsets", "dest", outputFile)
	f, err := os.Create(outputFile)
	if err != nil {
		logger.Error("failed to open output file", "error", err, "dest", outputFile)
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(to); err != nil {
		logger.Error("failed to write offsets", "error", err, "dest", outputFile)
		return err
	}
	return nil
}
