package challenge

import _ "embed"

//go:embed sql/find_challenge.sql

var findChallengeSQL string

//go:embed sql/find_challenge_locked.sql

var findChallengeLockedSQL string

//go:embed sql/revoke_challenges.sql

var revokeChallengesSQL string

//go:embed sql/insert_challenge.sql

var insertChallengeSQL string

//go:embed sql/save_challenge.sql

var saveChallengeSQL string
