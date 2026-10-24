const AUTH_ROUTES = [ "/login", "/register", "/verify-code", "/forgot-password", "/reset-password" ];


let loginRedirectPending = false;


const isAuthRoute = ( pathname: string ): boolean => AUTH_ROUTES.includes(pathname.replace(/\/+$/, "") || "/");


const isPublicRoute = ( pathname: string ): boolean => {
    return isAuthRoute(pathname) || [ "/personal-data-consent", "/privacy-policy" ].includes(pathname.replace(/\/+$/, ""));
};


const redirectToLogin = () => {
    if (typeof window === "undefined" || isPublicRoute(window.location.pathname) || loginRedirectPending) {
        return;
    }

    loginRedirectPending = true;
    window.location.replace("/login");
};


export { isAuthRoute, isPublicRoute, redirectToLogin };
