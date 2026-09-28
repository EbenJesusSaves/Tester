import { getPackageJson } from '@expo/config';

export function isExpoInstalled(projectDir: string): boolean {
  const packageJson = getPackageJson(projectDir);
  return !!(packageJson.dependencies && 'expo' in packageJson.dependencies);
}
