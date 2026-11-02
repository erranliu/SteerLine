import { afterEach, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import type { QueryClient } from "@tanstack/react-query";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, testClient } from "../../test/workspace";
import {
  useEnvironmentReadiness,
  type EnvironmentOptions,
} from "./use-readiness";

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((client) => client.clear());
  vi.restoreAllMocks();
});

function Probe() {
  const state = useEnvironmentReadiness();
  return (
    <>
      <output data-testid="required">{String(state.required)}</output>
      <output data-testid="verified">{String(state.verified)}</output>
      <output data-testid="observed">{String(state.observed)}</output>
      <output data-testid="profile">{state.data?.default_profile_id}</output>
    </>
  );
}

function options(required: boolean): EnvironmentOptions {
  return {
    environment_readiness_required: required,
    profiles: [],
    environments: [],
    default_profile_id: "default",
  };
}

function setup(
  configuration: () => Promise<EnvironmentOptions>,
  observation: () => Promise<EnvironmentOptions>,
  online = true,
  cached = false,
) {
  const api = new Api(identity, "csrf", "policy-home");
  const client = testClient();
  clients.push(client);
  const configurationKey = [
    api.home,
    identity.project_id,
    identity.instance_id,
    "environment",
    "configuration",
  ];
  if (cached) client.setQueryData(configurationKey, options(false));
  vi.spyOn(api, "get").mockImplementation(
    (path) =>
      (path.includes("configuration=true")
        ? configuration()
        : observation()) as never,
  );
  mountFeature(<Probe />, api, client, online);
  return { api, client, configurationKey };
}

const pending = () => new Promise<EnvironmentOptions>(() => {});

it("uses the frozen configuration policy while qualification is pending without inventing readiness", async () => {
  setup(async () => options(false), pending);
  await waitFor(() =>
    expect(screen.getByTestId("required")).toHaveTextContent("false"),
  );
  expect(screen.getByTestId("verified")).toHaveTextContent("false");
  expect(screen.getByTestId("observed")).toHaveTextContent("false");
  expect(screen.getByTestId("profile")).toHaveTextContent("default");
});

it("prefers the confirmed configuration policy over a conflicting observation", async () => {
  setup(
    async () => options(true),
    async () => options(false),
  );
  await waitFor(() =>
    expect(screen.getByTestId("verified")).toHaveTextContent("true"),
  );
  expect(screen.getByTestId("required")).toHaveTextContent("true");
});

it("can use a confirmed qualification response when the configuration read fails", async () => {
  setup(
    async () => {
      throw new Error("configuration unavailable");
    },
    async () => options(false),
  );
  await waitFor(() =>
    expect(screen.getByTestId("verified")).toHaveTextContent("true"),
  );
  expect(screen.getByTestId("required")).toHaveTextContent("false");
});

it("does not authorize a bypass from an offline cached configuration", () => {
  const { api } = setup(pending, pending, false, true);
  expect(screen.getByTestId("required")).toHaveTextContent("true");
  expect(api.get).not.toHaveBeenCalled();
});

it("restores the conservative gate when only an unverified cached policy remains", async () => {
  const { client, configurationKey } = setup(
    async () => {
      throw new Error("configuration unavailable");
    },
    pending,
    true,
    true,
  );
  await waitFor(() =>
    expect(screen.getByTestId("required")).toHaveTextContent("false"),
  );
  await client.refetchQueries({ queryKey: configurationKey, exact: true });
  await waitFor(() =>
    expect(screen.getByTestId("required")).toHaveTextContent("true"),
  );
  expect(
    client.getQueryData<EnvironmentOptions>(configurationKey)
      ?.environment_readiness_required,
  ).toBe(false);
});
