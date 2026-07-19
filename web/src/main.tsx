import { createRoot } from 'react-dom/client';
import App from './App';
import { LoginPage } from './Auth';
import CredentialUpdatePage from './CredentialUpdatePage';
import './styles.css';
import './recall-nexus.css';
import './components/recall/recall-explorer.css';
import './theme.css';

const pathname = window.location.pathname;
const isLoginPage = pathname === '/login';
const isCredentialUpdatePage = pathname === '/change-password';

const page = isLoginPage
  ? <LoginPage />
  : isCredentialUpdatePage
    ? <CredentialUpdatePage />
    : <App />;

createRoot(document.getElementById('root')!).render(page);
