import { useEffect } from 'react';
import { logout } from '@/lib/auth.ts';
import { useNavigate } from 'react-router';

export const Logout = () => {
  const navigate = useNavigate();

  useEffect(() => {
    logout();
    navigate('/login');
  }, [navigate]);

  return null;
};
