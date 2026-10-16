import { useEffect } from 'react';
import App from './App.tsx';
import { HomePage } from './HomePage.tsx';
import { SiteNav } from './SiteNav.tsx';
import { electricityPath, usePathname } from './navigation';

export function Site() {
  const pathname = usePathname();
  const electricity = pathname === electricityPath;

  useEffect(() => {
    document.title = electricity ? 'U.S. Electricity Mix' : 'American Trends';
  }, [electricity]);

  return (
    <>
      <SiteNav pathname={pathname} />
      {electricity ? <App /> : <HomePage />}
    </>
  );
}
