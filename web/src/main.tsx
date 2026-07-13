import { createRoot } from 'react-dom/client';
import App from './App';
import { LoginPage } from './Auth';
import CredentialUpdatePage from './CredentialUpdatePage';
import './styles.css';
import './recall-nexus.css';
import './components/recall/recall-explorer.css';
import './theme.css';

const page = window.location.pathname === '/login'
  ? <LoginPage />
  : window.location.pathname === '/change-password'
    ? <CredentialUpdatePage />
    : <App />;

createRoot(document.getElementById('root')!).render(page);
