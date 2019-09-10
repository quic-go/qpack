#!/bin/bash
set -ex

if [ "$CIRCLE_PULL_REQUEST" = "false" ]; then
    export FUZZING_TYPE="fuzzing"
    export BRANCH=${CIRCLE_BRANCH}
else
    export FUZZING_TYPE="local-regression"
    export BRANCH="PR-${CIRCLE_PULL_REQUEST}"
fi

## Install fuzzit
wget -q -O fuzzit https://github.com/fuzzitdev/fuzzit/releases/download/v2.4.44/fuzzit_Linux_x86_64
chmod a+x fuzzit

## Install go-fuzz
go get -u github.com/dvyukov/go-fuzz/go-fuzz github.com/dvyukov/go-fuzz/go-fuzz-build

cd fuzzing
go-fuzz-build -libfuzzer -o fuzz-qpack.a .
clang-9 -fsanitize=fuzzer fuzz-qpack.a -o fuzz-qpack
cd ..

# Create the jobs
./fuzzit create job --type ${FUZZING_TYPE} --branch ${BRANCH} --revision=${CIRCLE_SHA1} quic-go/fuzz-header fuzzing/fuzz-qpack
