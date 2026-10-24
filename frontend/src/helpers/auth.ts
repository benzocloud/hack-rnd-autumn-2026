const AUTH_ROUTES = [ "/login", "/register", "/verify-code", "/forgot-password", "/reset-password" ];


let loginRedirectPending = false;


const isAuthRoute = ( pathname: string ): boolean => AUTH_ROUTES.includes(pathname.replace(/\/+$/, "") || "/");


const canVerifyEmail = ( pathname: string, user: { email_verified: boolean } | null ): boolean => {
    return pathname.replace(/\/+$/, "") === "/verify-code" && user?.email_verified === false;
};


const isPublicRoute = ( pathname: string ): boolean => {
    return isAuthRoute(pathname) || [ "/personal-data-consent", "/privacy-policy" ].includes(pathname.replace(/\/+$/, ""));
};


const isKnownRoute = ( pathname: string ): boolean => {
    const path = pathname.replace(/\/+$/, "") || "/";
    return isPublicRoute(path) || [ "/", "/workspace", "/account" ].includes(path) || /^\/companies\/[^/]+\/(dashboard|settings|transactions|calendar|scenarios|imports|members|forecasts\/[^/]+)$/.test(path);
};


const redirectToLogin = () => {
    if (typeof window === "undefined" || isPublicRoute(window.location.pathname) || loginRedirectPending) {
        return;
    }

    loginRedirectPending = true;
    window.location.replace("/login");
};


export { isAuthRoute, isPublicRoute, isKnownRoute, canVerifyEmail, redirectToLogin };
