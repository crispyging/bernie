package serving

import (
	"fmt"
	"io"
	"strings"

	"github.com/opengfx1030/bernie/internal/app"
	"github.com/opengfx1030/bernie/internal/cli"
)

func init() {
	cmd := &cli.Command{Name: "chat", Summary: "Send one prompt to the running server and print the answer",
		Usage: `[--run NAME] [--max-tokens N] "PROMPT"`}
	cmd.Run = func(c *cli.Context, args []string) error {
		fs := c.Flags(cmd)
		run := fs.String("run", "", "run name (default: the run that is serving)")
		maxTokens := fs.Int("max-tokens", 512, "the most tokens of the answer")
		rest, err := cli.Parse(fs, args)
		if err != nil {
			return err
		}
		prompt := strings.TrimSpace(strings.Join(rest, " "))
		if prompt == "" {
			return cli.Usagef(`chat needs a prompt: bernie chat "Hello"`)
		}
		// The answer streams to stdout while it comes; the reasoning of a
		// model that sends one goes to stderr. With --json nothing streams.
		var onDelta func(app.ChatDelta) error
		if !c.JSON {
			onDelta = func(d app.ChatDelta) error {
				if d.Reasoning != "" {
					fmt.Fprint(c.Err, d.Reasoning)
				}
				_, err := fmt.Fprint(c.Out, d.Text)
				return err
			}
		}
		res, err := app.Chat(c.Ctx, *run, prompt, *maxTokens, onDelta)
		if err != nil {
			return err
		}
		return c.Emit(res, func(w io.Writer) {
			fmt.Fprintln(w)
			fmt.Fprintf(c.Err, "# %s (%s): first token %d ms, %d tokens, %.1f tok/s\n",
				res.Run, res.Model, res.TTFTMS, res.OutputTokens, res.TokS)
		})
	}
	cli.Register(cmd)
}
