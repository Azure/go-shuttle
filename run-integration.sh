#!/usr/bin/env bash

go get github.com/jstemmer/go-junit-report
go test -timeout 30m --tags=integration,debug -v ./integration | tee >(go-junit-report > integration.junit.xml)
