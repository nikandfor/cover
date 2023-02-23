module github.com/nikandfor/cover

go 1.17

require (
	github.com/nikandfor/assert v0.0.0-20220310091831-57b3fdb27159
	github.com/nikandfor/cli v0.0.0-20230223174857-3087496a8425
	github.com/nikandfor/errors v0.7.0
	github.com/nikandfor/escape v0.0.0-20211015113450-0e8be7818ccf
	github.com/nikandfor/tlog v0.14.1
	golang.org/x/mod v0.8.0
	golang.org/x/term v0.5.0
	golang.org/x/tools v0.6.0
)

require (
	github.com/nikandfor/loc v0.4.0 // indirect
	golang.org/x/sys v0.5.0 // indirect
)

//replace github.com/nikandfor/assert => ../assert

//replace github.com/nikandfor/tlog => ../tlog
