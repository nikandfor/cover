package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/escape/color"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/ext/tlflag"
	"github.com/nikandfor/tlog/low"
	"github.com/nikandfor/tlog/tlio"
	"golang.org/x/mod/modfile"
	"golang.org/x/term"
	"golang.org/x/tools/cover"
)

func main() {
	cli.App = cli.Command{
		Name:   "cover",
		Args:   cli.Args{},
		Before: before,
		Action: run,
		Flags: []*cli.Flag{
			cli.NewFlag("profile,p", "cover.out", "cover profile"),
			//	cli.NewFlag("diff,d", "", "compare to profile"),
			cli.NewFlag("color", false, "colorize output"),

			cli.NewFlag("log", "stderr", "log output file (or stderr)"),
			cli.NewFlag("verbosity,v", "", "logger verbosity topics"),

			cli.FlagfileFlag,
			cli.HelpFlag,
		},
	}

	cli.RunAndExit(os.Args)
}

func before(c *cli.Command) error {
	w, err := tlflag.OpenWriter(c.String("log"))
	if err != nil {
		return errors.Wrap(err, "open log file")
	}

	tlog.DefaultLogger = tlog.New(w)

	tlog.SetFilter(c.String("verbosity"))

	return nil
}

func run(c *cli.Command) (err error) {
	colorize := false

	fd := tlio.Fd(os.Stdout)
	if term.IsTerminal(int(fd)) {
		colorize = true
	}

	if f := c.Flag("color"); f.IsSet {
		colorize = c.Bool("color")
	}

	_ = colorize

	r, rel, err := root(c)
	if err != nil {
		return errors.Wrap(err, "find project root")
	}

	mod, err := module(c, r)
	if err != nil {
		return errors.Wrap(err, "determine module name")
	}

	//	tlog.Printw("paths", "root", r, "rel", rel, "module", mod)

	ps, err := cover.ParseProfiles(c.String("profile"))
	if err != nil {
		return errors.Wrap(err, "parse profile")
	}

	type stat struct {
		NumStmt int
		Covered float64
	}

	stats := map[string]*stat{}
	dirs := map[string]map[string]struct{}{}

	for _, p := range ps {
		total := 0
		covered := 0.

		for _, b := range p.Blocks {
			total += b.NumStmt
			if b.Count != 0 {
				covered += float64(b.NumStmt)
			}
		}

		covered /= float64(total)

		k := p.FileName
		for {
			s, ok := stats[k]
			if !ok {
				s = &stat{}
				stats[k] = s
			}

			s.NumStmt += total
			s.Covered += covered

			if k == mod {
				break
			}

			k = path.Dir(k)

			dir, ok := dirs[k]
			if !ok {
				dir = make(map[string]struct{})
				dirs[k] = dir
			}

			dir[p.FileName] = struct{}{}
		}
	}

	gray := color.New(90)
	green := color.New(color.Green)
	red := color.New(color.Red)
	reset := color.New(color.Reset)

	matcher := func(n string) bool { return true }

	if c.Args.Len() != 0 {
		matcher = newMatcher(mod, rel, c.Args)
	}

	for _, p := range ps {
		if !matcher(p.FileName) {
			continue
		}

		fmt.Printf("// %v\n", p.FileName)

		s := stats[p.FileName]
		fmt.Printf("// coverage %3.1f%%  mode:%v\n", 100*s.Covered, p.Mode)

		rel, err := filepath.Rel(mod, p.FileName)
		if err != nil {
			return errors.Wrap(err, "get relative")
		}

		src, err := os.ReadFile(filepath.Join(r, rel))
		if err != nil {
			return errors.Wrap(err, "read source file")
		}

		bs := p.Boundaries(src)

		if len(bs) == 0 {
			fmt.Printf("// no data\n")
			continue
		}

		var buf low.Buf

		if bs[0].Offset != 0 {
			buf = append(buf, gray...)
		}

		last := 0

		for i, b := range bs {
			buf = append(buf, src[last:b.Offset]...)
			last = b.Offset

			if !b.Start {
				if i+1 < len(bs) && bs[i+1].Offset == b.Offset {
					continue
				}

				buf = append(buf, gray...)

				continue
			}

			if b.Norm >= 0.5 {
				buf = append(buf, green...)
			} else {
				buf = append(buf, red...)
			}
		}

		if last != len(src) {
			buf = append(buf, src[last:]...)
		}

		buf.NewLine()

		buf = append(buf, reset...)

		fmt.Printf("%s", buf)
	}

	return nil
}

func root(c *cli.Command) (string, string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", "", errors.Wrap(err, "get workdir")
	}

	d := wd

	for d != "/" {
		_, err = os.Lstat(filepath.Join(d, "go.mod"))
		if err == nil {
			rel, err := filepath.Rel(d, wd)
			if err != nil {
				return "", "", errors.Wrap(err, "get relative path")
			}

			return d, rel, nil
		}

		d = filepath.Dir(d)
	}

	return "", "", errors.New("no go.mod found, project root can't be determined")
}

func module(c *cli.Command, r string) (string, error) {
	f := filepath.Join(r, "go.mod")

	data, err := os.ReadFile(f)
	if err != nil {
		return "", errors.Wrap(err, "no go.mod file")
	}

	gm, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return "", errors.Wrap(err, "parse go.mod")
	}

	return gm.Module.Mod.Path, nil
}

func newMatcher(mod, rel string, patterns []string) func(string) bool {
	if len(patterns) == 0 {
		return func(string) bool { return true }
	}

	type m struct {
		Path string
		Dots bool
	}

	ms := make([]m, len(patterns))

	for i, p := range patterns {
		dots := strings.HasSuffix(p, "...")

		if dots {
			p = strings.TrimSuffix(p, "...")
		}

		p = path.Join(mod, rel, p)

		p = path.Clean(p)

		tlog.V("match").Printw("pattern", "mod", mod, "rel", rel, "pattern", p)

		ms[i] = m{
			Path: p,
			Dots: dots,
		}
	}

	return func(n string) (ok bool) {
		if tlog.If("match") {
			defer func() {
				tlog.Printw("match", "ok", ok, "name", n)
			}()
		}

		dir := path.Dir(n)

		for _, m := range ms {
			if m.Dots {
				if strings.HasPrefix(dir, m.Path) {
					return true
				}
			} else {
				if dir == m.Path || n == m.Path {
					return true
				}

				if ok, _ := path.Match(m.Path, n); ok {
					return true
				}
			}
		}

		return false
	}
}
