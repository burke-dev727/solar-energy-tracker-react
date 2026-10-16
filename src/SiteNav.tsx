import { homePath, navItems, onNavClick } from './navigation';

interface SiteNavProps {
  pathname: string;
}

export function SiteNav({ pathname }: SiteNavProps) {
  return (
    <header className="site-nav">
      <a className="nav-brand" href={homePath} onClick={(event) => onNavClick(event, homePath)}>
        American Trends
      </a>
      <nav aria-label="Sections">
        <ul className="nav-links">
          {navItems.map((item) => (
            <li key={item.href}>
              <a
                href={item.href}
                aria-current={pathname === item.href ? 'page' : undefined}
                onClick={(event) => onNavClick(event, item.href)}
              >
                {item.label}
              </a>
            </li>
          ))}
        </ul>
      </nav>
    </header>
  );
}
