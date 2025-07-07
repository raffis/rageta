package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"slices"

	"github.com/raffis/rageta/pkg/apis/package/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
)

type Database struct {
	store   v1beta1.Store
	encoder kruntime.Serializer
}

type dbGetter interface {
	Get(name string) ([]byte, error)
}

func WithLocalDB(db dbGetter) Resolver {
	return func(ctx context.Context, ref string) (io.Reader, error) {
		b, err := db.Get(ref)
		return bytes.NewReader(b), err
	}
}

func OpenDatabase(r io.Reader, decoder kruntime.Decoder, encoder kruntime.Serializer) (*Database, error) {
	manifest, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	store := v1beta1.Store{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Store",
			APIVersion: v1beta1.GroupVersion.String(),
		},
	}

	_, _, err = decoder.Decode(
		manifest,
		nil,
		&store)

	database := &Database{
		store:   store,
		encoder: encoder,
	}

	return database, err
}

func (d *Database) Remove(name string) error {
	if !d.has(name) {
		return fmt.Errorf("no such pipeline found in local db store: %q", name)
	}

	d.store.Apps = slices.DeleteFunc(d.store.Apps, func(cmp v1beta1.App) bool {
		return cmp.Name == name
	})

	return nil
}

func (d *Database) Add(name string, manifest []byte) error {
	d.store.Apps = slices.DeleteFunc(d.store.Apps, func(cmp v1beta1.App) bool {
		return cmp.Name == name
	})

	app := v1beta1.App{
		Name:        name,
		InstalledAt: metav1.Now(),
		Manifest:    manifest,
	}

	d.store.Apps = append(d.store.Apps, app)
	return nil
}

func (d *Database) has(name string) bool {
	for _, app := range d.store.Apps {
		if app.Name == name {
			return true
		}
	}

	return false
}

func (d *Database) Get(name string) ([]byte, error) {
	for _, app := range d.store.Apps {
		if app.Name == name && app.Manifest != nil {
			return app.Manifest, nil
		}
	}

	return nil, fmt.Errorf("no such pipeline found in local db store: %q", name)
}

func (d *Database) Persist(w io.Writer) error {
	return d.encoder.Encode(&d.store, w)
}
