package steerline

func testSQLite(s *Service) *SQLiteStore { return s.domainStore.(*SQLiteStore) }
