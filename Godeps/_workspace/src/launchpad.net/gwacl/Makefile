# Build, and run tests. Be careful of the ordering of flags here; get
# it wrong and things can disappear into the void without warning.
# Yes, both -v and -gocheck.v=true are needed.
check: examples
	go test -v ./... -gocheck.v=true

debug-test:
	go test -c -gcflags "-N -l"
	gdb gwacl.test
	$(RM) gwacl.test

all_source := $(shell find . -name '*.go' ! -name '*_test.go')

example_source := $(wildcard example/*/run.go)
example_binaries := $(patsubst %.go,%,$(example_source))

# Clean up binaries.
clean:
	$(RM) $(example_binaries)

# Reformat the source files to match our layout standards.
# This includes gofmt's "simplify" option to streamline the source code.
format:
	./utilities/format -s

# Build the examples (we have no tests for them).
examples: $(example_binaries)

%: %.go $(all_source)
	go build -o $@ $<

.PHONY: check clean format examples debug_check
