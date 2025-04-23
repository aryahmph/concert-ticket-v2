#!/bin/bash

TEST_PATHS=$(cat .testdir | tr '\n' ' ')

go test -coverprofile=coverage.out $TEST_PATHS

go tool cover -func=coverage.out
go tool cover -html=coverage.out -o coverage.html