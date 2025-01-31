package grpc

import (
	"context"
	"io"

	"github.com/raffis/rageta/pkg/apis/network/v1beta1"
	"golang.org/x/sync/errgroup"
)

func AttachStreams(ctx context.Context, stream v1beta1.Gateway_ExecClient, stdin io.Reader, stdout, stderr io.Writer) error {
	g := errgroup.Group{}
	//route stdin
	if stdin != nil {
		g.Go(func() error {
			defer func() {
				_ = stream.CloseSend()
			}()

			buf := make([]byte, 100000)
			for {
				num, err := stdin.Read(buf)
				if err == io.EOF {
					return nil
				}

				if err != nil {
					return err
				}

				chunk := buf[:num]

				if err := stream.Send(&v1beta1.ExecRequest{Stdin: chunk}); err != nil {
					return nil
				}
			}
		})
	}

	//demux stdout/stderr streams
	g.Go(func() error {
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				//close(done)
				return err
			}

			if err != nil {
				return err
			}

			switch resp.Kind {
			case v1beta1.StreamKind_STREAM_STDOUT:
				if stdout != nil {
					_, _ = stdout.Write(resp.Stream)
				}
			case v1beta1.StreamKind_STREAM_STDERR:
				if stderr != nil {
					_, _ = stderr.Write(resp.Stream)
				}
			}
		}
	})

	g.Go(func() error {
		<-ctx.Done()
		return ctx.Err()
	})

	e := g.Wait()
	return e
}
