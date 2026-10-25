SELECT id,forecast_id,base_forecast_id,name,created_at,payload FROM core_scenarios WHERE company_id=$1 AND account_id=$2 AND (($4::boolean=false AND id=$3) OR ($4::boolean=true AND forecast_id=$3));
