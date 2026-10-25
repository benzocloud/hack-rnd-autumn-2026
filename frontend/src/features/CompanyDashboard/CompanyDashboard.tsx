"use client";

import { CreateForecastButton } from "@/features/ForecastDetail/ForecastDetail";
import Dashboard from "@/features/Dashboard/Dashboard";
import { useCompany } from "@/features/CompanyShell/CompanyShell";


const CompanyDashboard = () => {
    const { company } = useCompany();

    return <Dashboard key={company.id} companyId={company.id} forecastAction={<CreateForecastButton />} />;
};


export default CompanyDashboard;
