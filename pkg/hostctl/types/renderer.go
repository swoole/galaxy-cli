package types

// Row represents a line in the unmanaged part of a hosts file.
type Row struct {
	Comment string
	Profile string
	Status  string
	IP      string
	Host    string
}
