import { QueryClient } from "@tanstack/react-query";

export function createWebQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
        refetchOnWindowFocus: "always",
        refetchIntervalInBackground: false,
        staleTime: 1000,
      },
      mutations: { retry: false },
    },
  });
}
