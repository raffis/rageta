package provider

import (
	"fmt"
	"io"

	"github.com/raffis/rageta/pkg/apis/utils/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var (
	scheme  *kruntime.Scheme
	decoder kruntime.Decoder
	factory serializer.CodecFactory
	wire    kruntime.Serializer
)

// wire = protobuf.NewSerializer(nil, kruntime.MultiObjectTyper{})

type database struct {
	store v1beta1.Store
}

func Open(r io.Reader) (*database, error) {
	manifest, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	store := v1beta1.Store{}
	_, _, err = decoder.Decode(
		manifest,
		nil,
		&store)

	database := &database{
		store: store,
	}

	return database, err
}

func (d *database) Add(name string, manifest []byte) error {
	app := v1beta1.App{
		Name:        name,
		InstalledAt: metav1.Now(),
		Manifest:    manifest,
	}

	d.store.Apps = append(d.store.Apps, app)
	return nil
}

func (d *database) has(name string) bool {
	for _, app := range d.store.Apps {
		if app.Name == name {
			return true
		}
	}

	return false
}

func (d *database) Get(name string, manifest []byte) ([]byte, error) {
	for _, app := range d.store.Apps {
		if app.Name == name {
			return app.Manifest, nil
		}
	}

	return nil, fmt.Errorf("no such pipeline found: %s", name)
}

func (d *database) Persist(w io.Writer) error {
	return wire.Encode(&d.store, w)
}
