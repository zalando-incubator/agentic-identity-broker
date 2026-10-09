package ports

type CredentialFileReader interface {
	// ReadClientID acquires only the current client ID, without accessing a secret path.
	ReadClientID(path string) (string, error)
	// ReadPair returns one revalidated pair, or neither value on an acquisition error.
	ReadPair(clientIDPath, clientSecretPath string) (clientID, clientSecret string, err error)
}
