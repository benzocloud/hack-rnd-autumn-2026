UPDATE core_imports SET status=$2,lease_until=$3,expires_at=$4,terminal_at=$5,token=$6,raw_deleted=$7,payload=$8 WHERE id=$1;
