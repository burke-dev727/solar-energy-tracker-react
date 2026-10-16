import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { Site } from './SiteShell.tsx';
import './App.css';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <Site />
  </StrictMode>,
);
