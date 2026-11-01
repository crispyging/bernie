package serving

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
)

func init() {
	cmd := &cli.Command{Name: "connect", Summary: "Print the config an app needs to use the running server",
		Usage: "[" + strings.Join(app.ConnectApps(), "|") + "] [--run NAME]"}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		run := fs.String("run", "", "run name (default: the run that is serving)")
		rest, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		if len(rest) > 1 {
			return cli.Usagef("connect takes at most one app name")
		}
		o := app.ConnectOptions{Run: *run}
		if len(rest) == 1 {
			o.App = rest[0]
			if !slices.Contains(app.ConnectApps(), o.App) {
				return cli.Usagef("unknown app %q: use one of %s", o.App, strings.Join(app.ConnectApps(), ", "))
			}
		}
		res, err := app.Connect(c.Ctx, o)
		if err != nil {
			return err
		}
		return c.Emit(res, func(w io.Writer) {
			// The config alone goes to stdout, so it can be redirected into
			// a file; where it goes is a hint on stderr.
			fmt.Fprintf(c.Err, "# %s\n# run %s, model %s, base URL %s\n", res.Where, res.Run, res.Model, res.BaseURL)
			fmt.Fprint(w, res.Config)
		})
	}
	cli.Register(cmd)
}
