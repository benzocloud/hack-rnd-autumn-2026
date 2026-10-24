package user

import _ "embed"

//go:embed sql/insert_user.sql
var insertUserSQL string

//go:embed sql/user_by_email.sql
var userByEmailSQL string

//go:embed sql/identity_by_email.sql
var identityByEmailSQL string

//go:embed sql/identity_by_id.sql
var identityByIdSQL string

//go:embed sql/identity_by_id_locked.sql
var identityByIdLockedSQL string

//go:embed sql/insert_identity.sql
var insertIdentitySQL string

//go:embed sql/insert_credential.sql
var insertCredentialSQL string

//go:embed sql/set_email_verified.sql
var setEmailVerifiedSQL string

//go:embed sql/record_verification_failure.sql
var recordVerificationFailureSQL string
