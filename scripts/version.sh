#!/bin/sh
set -eu

if ! git rev-parse --verify HEAD >/dev/null 2>&1; then
	printf '%s\n' dev
	exit 0
fi

version=$(git describe --tags --always --match 'v[0-9]*' HEAD)
case "$version" in
	v*) version=${version#v} ;;
	*) version="dev-$version" ;;
esac

status=$(git status --porcelain --untracked-files=all)
if [ -n "$status" ]; then
	version="$version-dirty"
fi

printf '%s\n' "$version"
