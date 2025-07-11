package processor

type mockPipeline struct{}

func (m *mockPipeline) Step(name string) (Step, error) {
	return nil, nil
}

func (m *mockPipeline) Entrypoint(name string) (Next, error) {
	return nil, nil
}

func (m *mockPipeline) EntrypointName() (string, error) {
	return "", nil
}

func (m *mockPipeline) Name() string {
	return "mock"
}

func (m *mockPipeline) ID() string {
	return "mock-id"
}
