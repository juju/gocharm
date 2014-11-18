package main

import (
	"go/build"
	"io/ioutil"
	"path/filepath"
	"syscall"

	"github.com/juju/testing/filetesting"
	gc "gopkg.in/check.v1"
)

type suite struct{}

var _ = gc.Suite(suite{})

var copyContentsTests = []struct {
	pkg       string
	cwd       string
	pkgDir    filetesting.Entries
	destDir   filetesting.Entries
	assetFile string
}{{
	pkg: "arble.com/foo",
	pkgDir: filetesting.Entries{
		filetesting.Dir{"src", 0777},
		filetesting.Dir{"src/arble.com", 0777},
		filetesting.Dir{"src/arble.com/foo", 0777},
		filetesting.File{"src/arble.com/foo/foo.go", "package foo\n", 0666},
		filetesting.File{"src/arble.com/foo/bar.go", "package foo\n", 0666},
		filetesting.File{"src/arble.com/foo/README.md", "interesting stuff\n", 0666},
		filetesting.Dir{"src/arble.com/foo/assets", 0777},
		filetesting.File{"src/arble.com/foo/assets/file", "assiette", 0666},
	},
	destDir: filetesting.Entries{
		filetesting.Dir{"src", 0777},
		filetesting.Dir{"src/arble.com", 0777},
		filetesting.Dir{"src/arble.com/foo", 0777},
		filetesting.File{"src/arble.com/foo/foo.go", "package foo\n", 0666},
		filetesting.File{"src/arble.com/foo/bar.go", "package foo\n", 0666},
		filetesting.File{"src/arble.com/foo/README.md", "interesting stuff\n", 0666},
		filetesting.Dir{"src/arble.com/foo/assets", 0777},
		filetesting.File{"src/arble.com/foo/assets/file", "assiette", 0666},
		filetesting.Symlink{"assets", "src/arble.com/foo/assets"},
	},
	assetFile: "assiette",
}, {
	pkg: "arble.com/foo",
	pkgDir: filetesting.Entries{
		filetesting.Dir{"src", 0777},
		filetesting.Dir{"src/arble.com", 0777},
		filetesting.Dir{"src/arble.com/foo", 0777},
		filetesting.File{"src/arble.com/foo/foo.go", "package foo\n", 0666},
		filetesting.File{"src/arble.com/foo/bar.go", "package foo\n", 0666},
	},
	destDir: filetesting.Entries{
		filetesting.Dir{"src", 0777},
		filetesting.Dir{"src/arble.com", 0777},
		filetesting.Dir{"src/arble.com/foo", 0777},
		filetesting.File{"src/arble.com/foo/foo.go", "package foo\n", 0666},
		filetesting.File{"src/arble.com/foo/bar.go", "package foo\n", 0666},
	},
}}

func (suite) TestCopyContents(c *gc.C) {
	oldUmask := syscall.Umask(0)
	defer syscall.Umask(oldUmask)
	for i, test := range copyContentsTests {
		c.Logf("test %d", i)
		from := c.MkDir()
		test.pkgDir.Create(c, from)
		ctxt := build.Default
		ctxt.GOPATH = from
		pkg, err := ctxt.Import(test.pkg, test.cwd, 0)
		c.Assert(err, gc.IsNil)

		to := c.MkDir()
		err = copyContents(pkg, to)
		c.Assert(err, gc.IsNil)

		test.destDir.Check(c, to)

		if test.assetFile != "" {
			data, err := ioutil.ReadFile(filepath.Join(to, "assets", "file"))
			c.Assert(err, gc.IsNil)
			c.Assert(string(data), gc.Equals, test.assetFile)
		}
	}
}
