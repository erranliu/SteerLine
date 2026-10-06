package execution

// Identity returns the repository identity resolved and injected by the app.
func (c Config) Identity() string { return c.identity }

// BindRepositoryIdentity attaches the app-resolved identity to this frozen
// execution value.
func (c *Config) BindRepositoryIdentity(identity string) { c.identity = identity }

func (c Config) AcceptedConfigVersion() string  { return c.acceptedConfigVersion }
func (c Config) FixedProfileDigest() string     { return c.fixedProfileDigest }
func (c Config) FixedPublicationRemote() string { return c.publicationRemoteURL }

// BindAcceptedConfigVersion records the exact input digest selected by the
// app resolver; it does not carry the source paths used to compute that digest.
func (c *Config) BindAcceptedConfigVersion(version string) { c.acceptedConfigVersion = version }
