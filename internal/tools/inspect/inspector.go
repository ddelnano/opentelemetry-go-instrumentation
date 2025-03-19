// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package inspect

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/Masterminds/semver/v3"
	"github.com/docker/docker/client"
	"golang.org/x/sync/errgroup"

	"go.opentelemetry.io/auto/internal/pkg/funcfield"
	"go.opentelemetry.io/auto/internal/pkg/structfield"
)

const defaultNWorkers = 20

// Inspector inspects structure of Go packages.
type Inspector struct {
	NWorkers int
	Cache    *Cache

	log    *slog.Logger
	client *client.Client

	jobs []job
}

// New returns an Inspector that configured to inspect offsets defined in the
// manifests.
//
// If cache is non-nil, offsets will first be looked up there. Otherwise, the
// offsets will be found by building the applicatiions in the manifests and
// inspecting the produced binaries.
func New(logger *slog.Logger, cache *Cache, manifests ...Manifest) (*Inspector, error) {
	if cache == nil {
		logger.Info("using empty cache")
		cache = newCache(logger)
	}

	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}

	i := &Inspector{
		NWorkers: defaultNWorkers,
		log:      logger,
		Cache:    cache,
		client:   cli,
	}
	for _, m := range manifests {
		err := i.AddManifest(m)
		if err != nil {
			return nil, err
		}
	}
	return i, nil
}

// AddManifest adds the manifest to the Inspector's set of Manifests to
// inspect.
func (i *Inspector) AddManifest(manifest Manifest) error {
	if err := manifest.validate(); err != nil {
		return err
	}

	i.log.Debug("adding manifest", "manifest", manifest)

	goVer := manifest.Application.GoVerions
	if goVer == nil {
		// Passing nil to newBuilder will mean the application is built with
		// the latest version of Go.
		b := newBuilder(i.log, i.client, nil)
		for _, ver := range manifest.Application.Versions {
			v := ver
			if len(manifest.StructFields) > 0 && len(manifest.Funcs) > 0 {
				return errors.New("cannot use both struct fields and function fields in the same manifest")
			}
			i.jobs = append(i.jobs, job{
				Renderer: manifest.Application.Renderer,
				Builder:  b,
				AppVer:   v,
				Fields:   manifest.StructFields,
				Funcs:    manifest.Funcs,
			})
		}
		return nil
	}

	if manifest.Application.Versions == nil {
		for _, gVer := range goVer {
			v := gVer
			if len(manifest.StructFields) > 0 && len(manifest.Funcs) > 0 {
				return errors.New("cannot use both struct fields and function fields in the same manifest")
			}
			i.jobs = append(i.jobs, job{
				Renderer: manifest.Application.Renderer,
				Builder:  newBuilder(i.log, i.client, v),
				AppVer:   v,
				Fields:   manifest.StructFields,
				Funcs:    manifest.Funcs,
			})
		}
		return nil
	}

	for _, gV := range goVer {
		b := newBuilder(i.log, i.client, gV)
		for _, ver := range manifest.Application.Versions {
			v := ver
			if len(manifest.StructFields) > 0 && len(manifest.Funcs) > 0 {
				return errors.New("cannot use both struct fields and function fields in the same manifest")
			}
			i.jobs = append(i.jobs, job{
				Renderer: manifest.Application.Renderer,
				Builder:  b,
				AppVer:   v,
				Fields:   manifest.StructFields,
				Funcs:    manifest.Funcs,
			})
		}
	}
	return nil
}

type job struct {
	Renderer Renderer
	Builder  *builder
	AppVer   *semver.Version
	Fields   []structfield.ID
	Funcs    []funcfield.ID
}

type result struct {
	structs []stResult
	fns     []fnResult
}

// Do performs the inspections and returns all found offsets.
func (i *Inspector) Do(ctx context.Context) (*structfield.Index, error) {
	g, ctx := errgroup.WithContext(ctx)
	todo := make(chan job)

	g.Go(func() error {
		defer close(todo)
		for _, j := range i.jobs {
			select {
			case todo <- j:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	c := make(chan result)
	for n := 0; n < max(1, i.NWorkers-1); n++ {
		g.Go(func() error {
			for m := range todo {
				out, err := i.do(ctx, m)
				if err != nil {
					return err
				}

				select {
				case c <- out:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}
	go func() {
		_ = g.Wait()
		close(c)
	}()

	index := structfield.NewIndex()
	for results := range c {
		for _, r := range results.structs {
			i.logResult(r)

			index.PutOffset(r.StructField, r.Version, r.Offset, r.Valid)
		}
		for _, r := range results.fns {
			i.logFuncResult(r)

			index.PutFuncOffset(r.FuncField, r.Version, r.Offset, r.Location, r.Valid)
		}
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}
	return index, nil
}

type stResult struct {
	StructField structfield.ID
	FuncField   funcfield.ID
	Location    funcfield.Location
	Version     *semver.Version
	Offset      uint64
	// Valid is true if the offset is valid for the struct field at the specified version.
	Valid bool
}

type fnResult struct {
	FuncField funcfield.ID
	Location  funcfield.Location
	Version   *semver.Version
	Offset    uint64
	// Valid is true if the offset is valid for the struct field at the specified version.
	Valid bool
}

func (i *Inspector) do(ctx context.Context, j job) (out result, err error) {
	var uncachedStructIndices []int
	var uncachedFuncIndices []int
	for _, f := range j.Fields {
		o, ok := i.Cache.GetOffset(j.AppVer, f)
		out.structs = append(out.structs, stResult{
			StructField: f,
			Version:     j.AppVer,
			Offset:      o.Offset,
			Valid:       o.Valid,
		})
		if !ok {
			uncachedStructIndices = append(uncachedStructIndices, len(out.structs)-1)
		}
	}

	for _, f := range j.Funcs {
		// TODO(ddelnano): Add caching later if its warranted
		o, ok := i.Cache.GetFuncOffset(j.AppVer, f)
		out.fns = append(out.fns, fnResult{
			FuncField: f,
			Version:   j.AppVer,
			Offset:    o.Offset,
			Valid:     o.Valid,
			Location:  o.Location,
		})
		if !ok {
			uncachedFuncIndices = append(uncachedFuncIndices, len(out.fns)-1)
		}
	}
	if len(uncachedStructIndices) == 0 && len(uncachedFuncIndices) == 0 {
		return out, nil
	}

	app, err := newApp(ctx, i.log, j)
	buildErr := &errBuild{}
	if errors.As(err, &buildErr) {
		i.log.Debug(
			"failed to build app, skipping",
			"version", j.AppVer,
			"src", j.Renderer.src,
			"Go", j.Builder.GoImage,
			"rc", buildErr.ReturnCode,
			"stdout", buildErr.Stdout,
			"stderr", buildErr.Stderr,
		)
		return out, nil
	} else if err != nil {
		return out, err
	}
	defer app.Close()

	for _, i := range uncachedStructIndices {
		out.structs[i].Offset, out.structs[i].Valid = app.GetOffset(out.structs[i].StructField)
	}

	for _, i := range uncachedFuncIndices {
		fmt.Printf("uncachedFuncIndices: %d for %+v\n", i, out.fns[i].FuncField)
		out.fns[i].Offset, out.fns[i].Location, out.fns[i].Valid = app.GetFuncArgs(out.fns[i].FuncField)
	}

	return out, nil
}

func (i *Inspector) logResult(r stResult) {
	msg := "offset "
	kv := []interface{}{"version", r.Version, "id", r.StructField}
	if !r.Valid {
		msg += "not found"
	} else {
		msg += "found"
		kv = append(kv, "offset", r.Offset)
	}
	i.log.Info(msg, kv...)
}

func (i *Inspector) logFuncResult(r fnResult) {
	msg := "offset "
	kv := []interface{}{"version", r.Version, "id", r.FuncField, "location", r.Location}
	if !r.Valid {
		msg += "not found"
	} else {
		msg += "found"
		kv = append(kv, "offset", r.Offset)
	}
	i.log.Info(msg, kv...)
}
