package server

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitForDevelopmentBackend_waitsForReadiness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		probeStarted := make(chan struct{})
		backendReady := make(chan struct{})
		result := make(chan error, 1)
		child := &fakeDevelopmentChild{done: make(chan error)}
		go func() {
			result <- waitForDevelopmentBackend(
				t.Context(),
				&url.URL{},
				child,
				func(ctx context.Context, _ *url.URL) error {
					close(probeStarted)
					select {
					case <-backendReady:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				},
			)
		}()

		<-probeStarted
		synctest.Wait()
		select {
		case err := <-result:
			require.Failf(t, "readiness returned early", "error: %v", err)
		default:
		}

		close(backendReady)
		require.NoError(t, <-result)
	})
}

func TestWaitForDevelopmentBackend_childExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		probeStarted := make(chan struct{})
		result := make(chan error, 1)
		child := &fakeDevelopmentChild{done: make(chan error, 1)}
		go func() {
			result <- waitForDevelopmentBackend(
				t.Context(),
				&url.URL{},
				child,
				func(ctx context.Context, _ *url.URL) error {
					close(probeStarted)
					<-ctx.Done()
					return ctx.Err()
				},
			)
		}()

		<-probeStarted
		child.done <- errors.New("vite exited")

		err := <-result
		require.Error(t, err)
		assert.ErrorContains(t, err, "web development process exited before readiness: vite exited")
	})
}

type fakeDevelopmentChild struct {
	done chan error
}

func (c *fakeDevelopmentChild) Done() <-chan error {
	return c.done
}

func (*fakeDevelopmentChild) Stop(context.Context) error {
	return nil
}
