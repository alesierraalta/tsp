package main

import "time"

// timeAfter is the wait the ticker uses; it is a package function so a test can replace it.
var timeAfter = func(d time.Duration) <-chan time.Time { return time.After(d) }
