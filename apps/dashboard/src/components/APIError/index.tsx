import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert.tsx';
import { AlertCircle } from 'lucide-react';

export const ApiError = ({ error }: { error: Error }) => {
  return (
    <Alert variant="destructive" className="w-max">
      <AlertCircle className="h-4 w-4" />
      <AlertTitle>An error occurred while fetching data</AlertTitle>
      <AlertDescription>{error.message}</AlertDescription>
    </Alert>
  );
};
