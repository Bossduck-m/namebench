namebench 2.0
=============
namebench provides personalized DNS server recommendations based on your
browsing history.

WARNING: This tool is in the midst of a major rewrite. The "master" branch is currently in experimental
form and currently lacks a user interface, nor does it support any command-line options.

For stable binaries, please see https://code.google.com/p/namebench/

What can one expect in namebench 2.0?

* Faster
* Simpler interface
* More comprehensive results
* CDN benchmarking
* DNSSEC support


BUILDING:
=========
Building requires Go 1.22+ to be installed: https://go.dev/dl/

```
git clone https://github.com/google/namebench.git
cd namebench
go mod tidy
go build ./...
```

You should have an executable named `namebench` in the current directory (platform-specific extension may apply, such as `.exe` on Windows).


RUNNING:
========
* End-user: run ./namebench, which should open up a UI window.
* Developer, run ./namebench_dev_server.sh for an auto-reloading webserver at http://localhost:9080/
