package service

import "strconv"

// strconvItoa renders an int as a decimal string without importing strconv in
// every command file.
func strconvItoa(n int) string { return strconv.Itoa(n) }
