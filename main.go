package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nikandfor/cli"
	"github.com/nikandfor/errors"
	"github.com/nikandfor/escape/color"
	"github.com/nikandfor/tlog"
	"github.com/nikandfor/tlog/ext/tlflag"
	"github.com/nikandfor/tlog/low"
	"golang.org/x/mod/modfile"
	"golang.org/x/term"
	"golang.org/x/tools/cover"
)

var app *cli.Command

var help = `file_or_func_filter is either file or func filter.

Filter started with ! unselects previously matched.

	pkg !pkg.Type

File filter is a glob. 'path/...' is also supported. All paths are relative.

	wire/...
	version.go

Func filter matches package (not subpackages), type or func.

	pkg
	path/to/pkg
	pkg.Type
	pkg.Func
	Type
	Func
	Func.func1

cov_filter is applied to funcs if file_or_func_filter matched.
if file_or_func_filter contains '...' cov_filter is checked against files, not funcs in file.
cov_filter is :<g|l|a|b><percent>

	version.go:g10  - all functions from version.go with coverage greater then 10%
	pkg.Func:l30    - Func function if its coverage is less than 30%
	./api/...:b50   - all files in the api folder with coverage below 50%
	Type:a10:b90    - Type functions with coverage above 10 and below 90 percent
	:b33.333        - All functions with coverate below 1/3
	...:a66.66      - All files with coverate above 2/3
`

func main() {
	color := term.IsTerminal(int(os.Stdout.Fd()))

	app = &cli.Command{
		Name:        "cover",
		Usage:       "[-p <cover.out>] [[!]file_or_func_filter[cov_filter ...] ...]",
		Description: "render coverage profile with colors and filters",
		Help:        help,

		Args:   cli.Args{},
		Before: before,
		Action: render,
		Flags: []*cli.Flag{
			cli.NewFlag("profile,coverprofile,p", "cover.out", "cover profile"),

			//	cli.NewFlag("diff,d", "", "compare to profile"),
			cli.NewFlag("color", color, "colorize output"),
			cli.NewFlag("exit-code", false, "set exit code to 1 if something selected"),
			cli.NewFlag("silent", false, "do not print the code, useful with --exit-code"),

			cli.NewFlag("funcs-only,func-only", false, "do not render non function declarations (vars, types, ...)"),
			cli.NewFlag("no-file-comment", false, "do not print file name and coverage"),
			cli.NewFlag("no-func-comment", false, "do not print func name and coverage"),

			cli.NewFlag("log", "stderr", "log output file"),
			cli.NewFlag("verbosity,v", "", "logger verbosity topics"),

			cli.FlagfileFlag,
			cli.HelpFlag,
		},
	}

	cli.RunAndExit(app, os.Args, os.Environ())
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

		Some bool

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
		cov, tot := 0, 0

		for _, f := range flist {
			tp := "file"
			if f.src == nil {
				tp = "dir"
			}

			tlog.Printw(tp, "coverage", f.Norm, "name", f.Name)

			cov += f.Covered
			tot += f.Total
		}

		tlog.Printw("total", "coverage", float64(cov)/float64(tot))
	}

	if tlog.If("funcs") {
		for _, f := range flist {
			for _, ff := range f.Funcs {
				tlog.Printw("func", "coverage", ff.Norm, "name", ff.Name)
			}
		}
	}

	for _, a := range c.Args {
		set := true

		if strings.HasPrefix(a, "!") {
			set = false
			a = a[1:]
		}

		a, top, bot, err := parseCoverage(a)
		if err != nil {
			return errors.Wrap(err, "parse coverage filter")
		}

		tlog.V("cov").Printw("coverage", "top", top, "bottom", bot, "a", a)

		if a == "..." {
			for _, f := range files {
				if f.Norm <= bot {
					continue
				}
				if f.Norm >= top {
					continue
				}

				for _, ff := range f.Funcs {
					ff.Selected = set
				}
			}

			continue
		}

		if a == "" {
			for _, f := range files {
				for _, ff := range f.Funcs {
					if ff.Norm <= bot {
						continue
					}
					if ff.Norm >= top {
						continue
					}

					ff.Selected = set
				}
			}

			continue
		}

		dots := strings.HasSuffix(a, "/...")
		p := strings.TrimSuffix(a, "...")

		p = path.Join(mod, rel, p)

		for _, f := range flist {
			n := f.Name

			//	ok := n == p || dir == p || dots && strings.HasPrefix(dir, p)

			dir := filepath.Dir(n)

			ok, err := filepath.Match(p, n)
			if err != nil {
				return fmt.Errorf("match file %q: %w", p, err)
			}

			if !ok {
				ok, err = filepath.Match(p, dir)
				if err != nil {
					return fmt.Errorf("match dir %q: %w", p, err)
				}
			}

			for q := n; dots && !ok && q != "."; q = filepath.Dir(q) {
				ok = p == q
			}

			tlog.V("match_file").Printw("match file", "match", ok, "set", set, "p", p, "file", n)

			if ok {
				if dots {
					if f.Norm <= bot {
						continue
					}
					if f.Norm >= top {
						continue
					}
				}

				for _, ff := range f.Funcs {
					if !dots {
						if ff.Norm <= bot {
							continue
						}
						if ff.Norm >= top {
							continue
						}
					}

					ff.Selected = set
				}

				continue
			}

			if dots || a == "." {
				continue
			}

			for _, ff := range f.Funcs {
				fn := ff.Name

				// ok :=  strings.Contains(fn, a)

				ok, err := matchType(a, fn)
				if err != nil {
					return fmt.Errorf("match type %q: %w", p, err)
				}

				tlog.V("match_type,match_func").Printw("match func", "match", ok, "set", set, "p", a, "func", fn)

				if ok {
					if ff.Norm <= bot {
						continue
					}
					if ff.Norm >= top {
						continue
					}

					ff.Selected = set
				}
			}
		}
	}

	if c.Args.Len() == 0 {
		for _, f := range files {
			for _, ff := range f.Funcs {
				ff.Selected = true
			}
		}
	}

	for _, f := range flist {
		for _, ff := range f.Funcs {
			if !ff.Selected {
				continue
			}

			f.Some = true

			break
		}
	}

	var buf low.Buf

	for _, p := range ps {
		f := files[p.FileName]

		tlog.V("file").Printw("file", "show", f.Some, "norm", f.Norm, "cov", f.Covered, "total", f.Total, "file", f.Name)

		if !f.Some {
			continue
		}

		if !c.Bool("no-file-comment") {
			buf = low.AppendPrintf(buf, "// %v\n", p.FileName)
			buf = low.AppendPrintf(buf, "// covered %v (%.1f%%) of %v statements\n", f.Covered, 100*f.Norm, f.Total)
		}

		last := 0

		if !c.Bool("funcs-only") && len(f.Funcs) != 0 && last != f.Funcs[0].Pos {
			last = f.Funcs[0].Pos
			buf = renderFile(buf, f.src, 0, last, p)
		}

		for _, ff := range f.Funcs {
			tlog.V("func").Printw("func", "show", ff.Selected, "norm", ff.Norm, "cov", ff.Covered, "total", ff.Total, "func", ff.Name)

			if !ff.Selected {
				last = ff.End
				continue
			}

			if !c.Bool("funcs-only") && last != ff.Pos {
				buf = renderFile(buf, f.src, last, ff.Pos, p)
			}

			if !c.Bool("no-func-comment") {
				buf = low.AppendPrintf(buf, "// %v\n", ff.Name)
				buf = low.AppendPrintf(buf, "// covered %v (%.1f%%) of %v statements\n", ff.Covered, 100*ff.Norm, ff.Total)
			}

			buf = renderFile(buf, f.src, ff.Pos, ff.End, p)

			last = ff.End
		}

		if !c.Bool("funcs-only") && last != f.Size {
			buf = renderFile(buf, f.src, last, f.Size, p)
		}
	}

	if !c.Bool("silent") {
		fmt.Fprintf(c, "%s", buf)
	}

	if c.Bool("exit-code") && len(buf) != 0 {
		os.Exit(1)
	}

	return nil
}

func renderFile(buf, src []byte, pos, end int, p *cover.Profile) []byte {
	var gray, green, red, reset []byte

	if app.Bool("color") {
		gray = color.New(90)
		green = color.New(color.Green)
		red = color.New(color.Red)
		reset = color.New(color.Reset)
	}

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

func parseCoverage(a string) (_ string, top, bot float64, err error) {
	top = 2.
	bot = -1.

	end := len(a)

	for {
		p := strings.LastIndexByte(a[:end], ':')
		if p == -1 {
			break
		}

		if p+2 >= len(a) {
			err = errors.New("no filter")
			return
		}

		var f float64
		f, err = strconv.ParseFloat(a[p+2:], 64)
		if err != nil {
			err = errors.Wrap(err, "percentage")
			return
		}

		f /= 100

		switch a[p+1] {
		case 'l', 'b':
			top = f
		case 'g', 'a':
			bot = f
		default:
			err = errors.New("unsupported operation: %q", a[p])
			return
		}

		end = p
	}

	return a[:end], top, bot, nil
}

func matchType(pattern, fn string) (ok bool, err error) {
	re, err := regexp.Compile(`([/\.]|^)` + pattern + `(\.|$)`)
	if err != nil {
		return false, err
	}

	ok = re.MatchString(fn)

	return ok, nil
}
