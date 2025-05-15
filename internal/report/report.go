package report

type Finalizer interface {
	Finalize() error
}
