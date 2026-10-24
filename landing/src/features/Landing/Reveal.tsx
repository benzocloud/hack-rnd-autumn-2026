"use client";

import { useEffect } from "react";


const Reveal = () => {
    useEffect(() => {
        if (window.matchMedia("(prefers-reduced-motion: reduce)").matches || !("IntersectionObserver" in window)) {
            return;
        }

        const observer = new IntersectionObserver(( entries ) => {
            entries.forEach(( entry ) => {
                if (entry.isIntersecting) {
                    (entry.target as HTMLElement).dataset.entered = "true";
                    observer.unobserve(entry.target);
                }
            });
        }, { threshold: 0.12 });

        document.querySelectorAll("[data-reveal]").forEach(( element ) => observer.observe(element));

        return () => observer.disconnect();
    }, []);

    return null;
};


export default Reveal;
