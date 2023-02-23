module github.com/nikandfor/cover

go 1.17

require (
	github.com/nikandfor/assert v0.0.0-20220304193615-0dc830fd5c42
	github.com/nikandfor/cli v0.0.0-20220304193817-d501836a0b4c
	github.com/nikandfor/errors v0.7.0
	github.com/nikandfor/escape v0.0.0-20211015113450-0e8be7818ccf
	github.com/nikandfor/tlog v0.14.1-0.20220304235114-e4946fd9cbdf
	golang.org/x/mod v0.5.1
	golang.org/x/term v0.0.0-20201126162022-7de9c90e9dd1
	golang.org/x/tools v0.1.9
)

require (
	github.com/nikandfor/loc v0.4.0 // indirect
	golang.org/x/sys v0.0.0-20211019181941-9d821ace8654 // indirect
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1 // indirect
)

//replace github.com/nikandfor/assert => ../assert

//replace github.com/nikandfor/tlog => ../tlog
