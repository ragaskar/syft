package etui

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/anchore/syft/internal/ui/common"
	"github.com/anchore/syft/ui"

	"github.com/anchore/syft/internal/log"
	syftEvent "github.com/anchore/syft/syft/event"
	"github.com/wagoodman/go-partybus"
	"github.com/wagoodman/jotframe/pkg/frame"
)

// TODO: specify per-platform implementations with build tags

func setupScreen(output *os.File) *frame.Frame {
	config := frame.Config{
		PositionPolicy: frame.PolicyFloatForward,
		// only report output to stderr, reserve report output for stdout
		Output: output,
	}

	fr, err := frame.New(config)
	if err != nil {
		log.Errorf("failed to create screen object: %+v", err)
		return nil
	}
	return fr
}

// nolint:funlen,gocognit
func OutputToEphemeralTUI(workerErrs <-chan error, subscription *partybus.Subscription) error {
	output := os.Stderr

	// hide cursor
	_, _ = fmt.Fprint(output, "\x1b[?25l")
	// show cursor
	defer fmt.Fprint(output, "\x1b[?25h")

	fr := setupScreen(output)
	if fr == nil {
		return fmt.Errorf("unable to setup screen")
	}
	var isClosed bool
	defer func() {
		if !isClosed {
			fr.Close()
			frame.Close()
		}
	}()

	var err error
	var wg = &sync.WaitGroup{}
	events := subscription.Events()
	ctx := context.Background()
	syftUIHandler := ui.NewHandler()

eventLoop:
	for {
		select {
		case err := <-workerErrs:
			if err != nil {
				return err
			}
		case e, ok := <-events:
			if !ok {
				break eventLoop
			}
			switch {
			case syftUIHandler.RespondsTo(e):
				if err = syftUIHandler.Handle(ctx, fr, e, wg); err != nil {
					log.Errorf("unable to show %s event: %+v", e.Type, err)
				}

			case e.Type == syftEvent.AppUpdateAvailable:
				if err = appUpdateAvailableHandler(ctx, fr, e, wg); err != nil {
					log.Errorf("unable to show %s event: %+v", e.Type, err)
				}

			case e.Type == syftEvent.CatalogerFinished:
				// we may have other background processes still displaying progress, wait for them to
				// finish before discontinuing dynamic content and showing the final report
				wg.Wait()
				fr.Close()
				frame.Close()
				isClosed = true

				if err := common.CatalogerFinishedHandler(e); err != nil {
					log.Errorf("unable to show %s event: %+v", e.Type, err)
				}

				// this is the last expected event
				break eventLoop
			}
		case <-ctx.Done():
			if ctx.Err() != nil {
				log.Errorf("cancelled (%+v)", err)
			}
			break eventLoop
		}
	}

	return nil
}