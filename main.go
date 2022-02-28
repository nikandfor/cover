package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/escape/color"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/ext/tlflag"
	"github.com/nikandfor/tlog/low"
	"golang.org/x/mod/modfile"
	"golang.org/x/tools/cover"
)

func main() {
	cli.App = cli.Command{
		Name:        "cover",
		Usage:       "[-p <cover.out>] [file_or_func_to_select ...]",
		Description: "colors source code by coverage profile",
		Args:        cli.Args{},
		Before:      before,
		Action:      render,
		Flags: []*cli.Flag{
			cli.NewFlag("profile,p", "cover.out", "cover profile"),
			//	cli.NewFlag("diff,d", "", "compare to profile"),
			//	cli.NewFlag("color", false, "colorize output"),

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

func render(c *cli.Command) (err error) {
	r, rel, err := root(c)
	if err != nil {
		return errors.Wrap(err, "find project root")
	}

	mod, err := module(c, r)
	if err != nil {
		return errors.Wrap(err, "determine module name")
	}

	ps, err := cover.ParseProfiles(c.String("profile"))
	if err != nil {
		return errors.Wrap(err, "parse profile")
	}

	if len(ps) == 0 {
		fmt.Fprintf(os.Stderr, "no coverage data\n")
		return
	}

	type fun struct {
		Name string

		Decl ast.Decl
		Pos  int
		End  int

		Covered int
		Total   int
		Norm    float64

		Selected bool
	}

	type file struct {
		Name string

		Pos  token.Pos
		Base int
		Size int

		Funcs []*fun
		funcs map[string]*fun

		Covered int
		Total   int
		Norm    float64

		Full bool

		src []byte
	}

	files := map[string]*file{}

	fset := token.NewFileSet()

	for _, p := range ps {
		rel, err := filepath.Rel(mod, p.FileName)
		if err != nil {
			return errors.Wrap(err, "get relative")
		}

		src, err := os.ReadFile(filepath.Join(r, rel))
		if err != nil {
			return errors.Wrap(err, "read source file")
		}

		fl, err := parser.ParseFile(fset, "", src, 0)
		if err != nil {
			return errors.Wrap(err, "parse source file")
		}

		tokFile := fset.File(fl.Pos())

		total := 0
		covered := 0

		for _, b := range p.Blocks {
			total += b.NumStmt
			if b.Count != 0 {
				covered += b.NumStmt
			}
		}

		pkg := path.Dir(p.FileName)

		var funlst []*fun
		funcs := map[string]*fun{}

		for _, d := range fl.Decls {
			f, ok := d.(*ast.FuncDecl)
			if !ok {
				continue
			}

			total := 0
			covered := 0

			for _, b := range p.Blocks {
				off := tokFile.LineStart(b.StartLine) + token.Pos(b.StartCol-1)
				end := tokFile.LineStart(b.EndLine) + token.Pos(b.EndCol-1)

				if off < f.Pos() || end > f.End() {
					continue
				}

				total += b.NumStmt
				if b.Count != 0 {
					covered += b.NumStmt
				}
			}

			var n string

			if f.Recv != nil {
				svc := expr(f.Recv.List[0].Type)

				n = pkg + "." + svc + "." + f.Name.Name
			} else {
				n = pkg + "." + f.Name.Name
			}

			ff := &fun{
				Name: n,
				Decl: d,

				Pos: tokFile.Offset(f.Pos()),
				End: tokFile.Offset(f.End()),

				Total:   total,
				Covered: covered,
			}

			funcs[n] = ff
			funlst = append(funlst, ff)
		}

		f := &file{
			Name:    p.FileName,
			Pos:     fl.Pos(),
			Base:    tokFile.Base(),
			Size:    tokFile.Size(),
			Funcs:   funlst,
			funcs:   funcs,
			src:     src,
			Total:   total,
			Covered: covered,
		}

		files[p.FileName] = f

		k := p.FileName

		for {
			k = filepath.Dir(k)

			dir, ok := files[k]
			if !ok {
				dir = &file{
					Name: k,
				}
				files[k] = dir
			}

			dir.Covered += f.Covered
			dir.Total += f.Total

			if filepath.ToSlash(k) == mod {
				break
			}
		}
	}

	flist := make([]*file, 0, len(files))
	for _, f := range files {
		flist = append(flist, f)
	}

	sort.Slice(flist, func(i, j int) (less bool) {
		li := strings.Split(flist[i].Name, string(rune(os.PathSeparator)))
		lj := strings.Split(flist[j].Name, string(rune(os.PathSeparator)))

		//	defer func() {
		//		tlog.Printw("cmp paths", "less", less, "i", flist[i].Name, "i_dir", flist[i].src == nil, "j", flist[j].Name, "j_dir", flist[j].src == nil)
		//	}()

		k := 0
		for k < len(li) && k < len(lj) && li[k] == lj[k] {
			k++
		}

		if k == len(lj) {
			return false
		}

		if k == len(li) {
			return true
		}

		return li[k] < lj[k]
	})

	for _, f := range flist {
		if f.Total != 0 {
			f.Norm = float64(f.Covered) / float64(f.Total)
		}

		for _, ff := range f.Funcs {
			if ff.Total != 0 {
				ff.Norm = float64(ff.Covered) / float64(ff.Total)
			}
		}
	}

	if tlog.If("files") {
		for _, f := range flist {
			tp := "file"
			if f.src == nil {
				tp = "folder"
			}

			tlog.Printw(tp, "coverage", f.Norm, "name", f.Name)
		}
	}

	if tlog.If("funcs") {
		for _, f := range flist {
			for _, ff := range f.Funcs {
				tlog.Printw("func", "coverage", ff.Norm, "name", ff.Name)
			}
		}
	}

	for _, a := range c.Args {
		dots := strings.HasSuffix(a, "...")
		p := strings.TrimSuffix(a, "...")

		p = path.Join(mod, rel, p)

		tlog.V("match").Printw("pattern", "p", p, "dots", dots)

		for n, f := range files {
			if f.Full {
				continue
			}

			dir := path.Dir(n)

			if n == p || dir == p || dots && strings.HasPrefix(dir, p) {
				f.Full = true
				continue
			}

			if dots || a == "." {
				continue
			}

			for _, ff := range f.Funcs {
				fn := ff.Name

				if ff.Selected {
					continue
				}

				if strings.Contains(fn, a) {
					ff.Selected = true
				}
			}
		}
	}

	if c.Args.Len() == 0 {
		for _, f := range files {
			f.Full = true
		}
	}

	var buf low.Buf

	for _, p := range ps {
		f := files[p.FileName]

		tlog.V("file").Printw("file", "cov", f.Covered, "total", f.Total, "full", f.Full, "file", f.Name)

		if f.Full {
			buf = low.AppendPrintf(buf, "// %v\n", p.FileName)
			buf = low.AppendPrintf(buf, "// covered %v (%.1f%%) of %v statements\n", f.Covered, 100*f.Norm, f.Total)

			buf = renderFile(buf, f.src, 0, f.Size, p)

			continue
		}

		for _, ff := range f.Funcs {
			tlog.V("func").Printw("func", "cov", ff.Covered, "total", ff.Total, "seld", ff.Selected, "func", ff.Name)

			if ff.Selected {
				buf = low.AppendPrintf(buf, "// %v\n", ff.Name)
				buf = low.AppendPrintf(buf, "// covered %v (%.1f%%) of %v statements\n", ff.Covered, 100*ff.Norm, ff.Total)

				buf = renderFile(buf, f.src, ff.Pos, ff.End, p)
			}
		}
	}

	fmt.Printf("%s", buf)

	return nil
}

func renderFile(buf, src []byte, pos, end int, p *cover.Profile) []byte {
	gray := color.New(90)
	green := color.New(color.Green)
	red := color.New(color.Red)
	reset := color.New(color.Reset)

	bs := p.Boundaries(src)

	last := pos

	i := 0
	for i < len(bs) && bs[i].Offset < pos {
		i++
	}

	if i < len(bs) && bs[i].Offset != pos {
		buf = append(buf, gray...)
	}

	for ; i < len(bs); i++ {
		b := bs[i]

		if b.Offset >= end {
			break
		}

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

	if last != end {
		buf = append(buf, src[last:end]...)
	}

	nl := len(buf) == 0 || buf[len(buf)-1] != '\n'

	buf = append(buf, reset...)

	if nl {
		buf = append(buf, '\n')
	}

	return buf
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

func expr(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.StarExpr:
		return expr(e.X)
	case *ast.Ident:
		return e.Name
	default:
		tlog.Printw("eval expr", "expr", e, "type", tlog.FormatNext("%T"), e)
		panic(e)
	}
}
