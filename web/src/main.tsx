import { createRoot } from 'react-dom/client';
import App from './App';
import { CredentialUpdatePage, LoginPage } from './Auth';
import './styles.css';
import './recall-nexus.css';

const page = window.location.pathname === '/login'
  ? <LoginPage />
  : window.location.pathname === '/change-password'
    ? <CredentialUpdatePage />
    : <App />;

createRoot(document.getElementById('root')!).render(page);
