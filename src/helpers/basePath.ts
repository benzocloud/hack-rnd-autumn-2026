const basePath = process.env.NEXT_PUBLIC_BASE_PATH || "";


const withBasePath = ( path: string ): string => `${basePath}${path}`;


const withoutBasePath = ( path: string ): string => {
    return basePath && (path === basePath || path.startsWith(`${basePath}/`)) ? path.slice(basePath.length) || "/" : path;
};


export { withBasePath, withoutBasePath };
