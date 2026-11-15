package store

import "time"

func minutes(n int) time.Duration { return time.Duration(n) * time.Minute }
