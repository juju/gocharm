#!/bin/sh
sudo sh -c "
    export GOMAXPROCS=\"$GOMAXPROCS\"
    export GOPATH=\"$GOPATH\"
    export GOROOT=\"$GOROOT\"
    export PATH=\"$PATH\"
    go test $*
"