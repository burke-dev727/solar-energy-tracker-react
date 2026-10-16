import { useEffect } from 'react';
import App from './App.tsx';
import { DataCentersPage } from './DataCentersPage.tsx';
import { HomePage } from './HomePage.tsx';
import { SiteNav } from './SiteNav.tsx';
import { dataCentersPath, electricityPath, usePathname } from './navigation';

const pageTitles: Record<string, string> = {
  [electricityPath]: 'U.S. Electricity Mix',
  [dataCentersPath]: 'U.S. Data Centers',
};

export function Site() {
  const pathname = usePathname();

  useEffect(() => {
    document.title = pageTitles[pathname] ?? 'American Trends';
  }, [pathname]);

  return (
    <>
      <SiteNav pathname={pathname} />
      {pathname === electricityPath ? (
        <App />
      ) : pathname === dataCentersPath ? (
        <DataCentersPage />
      ) : (
        <HomePage />
      )}
    </>
  );
}
