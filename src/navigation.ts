import { useEffect, useState, type MouseEvent } from 'react';

export const homePath = '/';
export const electricityPath = '/electricity';

/** Sections in the bar. Add future pages here. */
export const navItems = [{ href: electricityPath, label: 'U.S. Electricity Mix' }] as const;

export function normalizePath(pathname: string) {
  if (pathname.length > 1 && pathname.endsWith('/')) {
    return pathname.slice(0, -1);
  }
  return pathname;
}

export function usePathname() {
  const [pathname, setPathname] = useState(() => normalizePath(window.location.pathname));

  useEffect(() => {
    const sync = () => setPathname(normalizePath(window.location.pathname));
    window.addEventListener('popstate', sync);
    return () => window.removeEventListener('popstate', sync);
  }, []);

  return pathname;
}

export function onNavClick(event: MouseEvent<HTMLAnchorElement>, href: string) {
  if (
    event.defaultPrevented ||
    event.button !== 0 ||
    event.metaKey ||
    event.ctrlKey ||
    event.shiftKey ||
    event.altKey
  ) {
    return;
  }
  event.preventDefault();
  if (normalizePath(window.location.pathname) === href) {
    return;
  }
  window.history.pushState(null, '', href);
  window.dispatchEvent(new PopStateEvent('popstate'));
}
