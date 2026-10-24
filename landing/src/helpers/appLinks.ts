const APP_URL = (process.env.NEXT_PUBLIC_APP_URL || "https://saldo.brokebyte.me").replace(/\/$/, "");

const appLinks = {
    login: `${APP_URL}/login`,
    register: `${APP_URL}/register`,
};


export default appLinks;

