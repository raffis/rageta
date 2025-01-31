package output

/*
func stdio(event processor.Event) (io.Reader, io.Writer, io.Writer) {
	var stdout io.Writer = os.Stdout
	var stderr io.Writer = os.Stderr
	var stdin io.Reader

	if event.Pod.Spec.Containers[0].Stdin {
		stdin = os.Stdin
	}

	return stdin, stdout, stderr
}

func attachJobStreams(event processor.Event, stdin io.Reader, stdout, stderr io.Writer) {
	fd := int(os.Stdin.Fd())
	if event.Pod.Spec.Containers[0].TTY && term.IsTerminal(fd) {
		var oldState *term.State
		oldState, err := term.MakeRaw(fd)
		if err != nil {
			//return err
		}

		defer term.Restore(fd, oldState)
	}

	event.Job.Attach(stdin, stdout, stderr)
}
*/
