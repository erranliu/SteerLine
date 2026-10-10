using System.Diagnostics;
using System.Globalization;
using System.Net;
using System.Net.Http.Headers;
using System.Security.AccessControl;
using System.Security.Principal;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using System.Text.Json.Serialization;
using System.Text.RegularExpressions;

namespace SteerLine.Windows.Core;

public interface IReadOnlyLocalService : IDisposable
{
    ServiceIdentity Identity { get; }
    string HomeDirectory { get; }
    Task<TaskPage> ListTasksAsync(CancellationToken cancellationToken);
    Task<TaskDetailPage> ShowTaskAsync(string taskId, CancellationToken cancellationToken);
}

public interface ILocalServiceFactory
{
    Task<IReadOnlyLocalService> ConnectAsync(string configPath, string projectPath, CancellationToken cancellationToken);
}

public sealed class LocalServiceFactory : ILocalServiceFactory
{
    private const string ClientProtocol = "steerline-client/v1";
    private const int MaxDiscoveryBytes = 64 * 1024;
    private const int MaxResponseBytes = 32 * 1024 * 1024;
    private static readonly JsonSerializerOptions JsonOptions = new() { PropertyNameCaseInsensitive = false };

    public async Task<IReadOnlyLocalService> ConnectAsync(string configPath, string projectPath, CancellationToken cancellationToken)
    {
        var configuration = ProjectConfiguration.Read(configPath);
        var selectedProject = CanonicalPath(projectPath);
        if (!PathEquals(configuration.RepositoryPath, selectedProject))
            throw new LocalServiceException("project_mismatch", "所选项目目录与配置文件中的 repository 不一致。");

        var commonDirectory = await ResolveCommonDirectoryAsync(configuration, cancellationToken).ConfigureAwait(false);
        var repositoryIdentity = CanonicalPath(commonDirectory);
        var projectId = ProjectId(repositoryIdentity);
        var runDirectory = configuration.RunDirectory ?? Path.Combine(configuration.HomeDirectory, "state", RepositoryStateKey(repositoryIdentity, projectId));
        var apiDirectory = Path.Combine(runDirectory, "local-api");
        var discoveryPath = Path.Combine(apiDirectory, "discovery.json");
        if (!File.Exists(discoveryPath))
            throw new LocalServiceException("service_offline", "没有找到该项目的现有服务发现文件。请从服务窗口启动服务后重试。");

        VerifyPrivateDirectory(apiDirectory);
        var discoveryBytes = await File.ReadAllBytesAsync(discoveryPath, cancellationToken).ConfigureAwait(false);
        if (discoveryBytes.Length == 0 || discoveryBytes.Length > MaxDiscoveryBytes)
            throw new LocalServiceException("invalid_discovery", "服务发现文件为空或超过 64 KiB。");
        VerifyPrivateFile(discoveryPath);
        var discovery = JsonSerializer.Deserialize<ServiceDiscovery>(discoveryBytes, JsonOptions)
            ?? throw new LocalServiceException("invalid_discovery", "服务发现文件不是有效的 JSON 对象。");

        ValidateDiscovery(discovery, projectId, repositoryIdentity, apiDirectory);
        VerifyPrivateDirectory(Path.Combine(apiDirectory, "credentials"));
        VerifyPrivateFile(discovery.CredentialPath);
        VerifyProcessIdentity(discovery.Process);
        var token = await File.ReadAllTextAsync(discovery.CredentialPath, cancellationToken).ConfigureAwait(false);
        if (token.Length != 64 || token.Any(c => !Uri.IsHexDigit(c)))
            throw new LocalServiceException("invalid_credential", "发现文件中的当前实例凭证长度或格式无效。");

        var handler = new HttpClientHandler { UseProxy = false, UseCookies = false, AllowAutoRedirect = false };
        var http = new HttpClient(handler, disposeHandler: true) { Timeout = TimeSpan.FromSeconds(10) };
        var client = new LocalServiceClient(http, discovery, token, configuration.HomeDirectory);
        try
        {
            var identity = await client.RefreshIdentityAsync(cancellationToken).ConfigureAwait(false);
            IdentityGuard.Verify(discovery, identity, projectId, repositoryIdentity);
            return client;
        }
        catch
        {
            client.Dispose();
            throw;
        }
    }

    private static void ValidateDiscovery(ServiceDiscovery discovery, string projectId, string repositoryIdentity, string apiDirectory)
    {
        if (discovery.Version != 1 || discovery.Protocol != ClientProtocol || discovery.ProjectId != projectId ||
            !PathEquals(discovery.RepositoryIdentity, repositoryIdentity) || string.IsNullOrWhiteSpace(discovery.InstanceId) ||
            string.IsNullOrWhiteSpace(discovery.LeaseId))
            throw new LocalServiceException("identity_conflict", "发现文件的协议、项目或实例身份与所选项目不匹配。");

        if (!IPAddress.TryParse(AddressHost(discovery.Address), out var ip) || !IPAddress.Loopback.Equals(ip) ||
            ip.AddressFamily != System.Net.Sockets.AddressFamily.InterNetwork || !ValidPort(discovery.Address))
            throw new LocalServiceException("invalid_discovery", "发现地址不是有效的 IPv4 loopback 端点。");

        var credentialDirectory = Path.GetFullPath(Path.Combine(apiDirectory, "credentials")) + Path.DirectorySeparatorChar;
        var credentialPath = Path.GetFullPath(discovery.CredentialPath);
        if (!credentialPath.StartsWith(credentialDirectory, StringComparison.OrdinalIgnoreCase))
            throw new LocalServiceException("identity_conflict", "当前实例凭证路径超出了本地 API credentials 目录。");
    }

    private static string AddressHost(string address)
    {
        var separator = address.LastIndexOf(':');
        return separator > 0 ? address[..separator] : string.Empty;
    }

    private static bool ValidPort(string address)
    {
        var separator = address.LastIndexOf(':');
        return separator > 0 && ushort.TryParse(address[(separator + 1)..], NumberStyles.None, CultureInfo.InvariantCulture, out var port) && port > 0;
    }

    private static async Task<string> ResolveCommonDirectoryAsync(ProjectConfiguration configuration, CancellationToken cancellationToken)
    {
        var startInfo = ProjectConfiguration.CreateIdentityStartInfo(configuration);

        using var process = new Process { StartInfo = startInfo };
        try
        {
            if (!process.Start())
                throw new LocalServiceException("project_identity_unavailable", "无法启动配置指定的 Git 可执行文件。");
        }
        catch (Exception exception) when (exception is not LocalServiceException)
        {
            throw new LocalServiceException("project_identity_unavailable", $"无法启动配置指定的 Git：{exception.Message}");
        }

        var stdout = process.StandardOutput.ReadToEndAsync(cancellationToken);
        var stderr = process.StandardError.ReadToEndAsync(cancellationToken);
        using var timeout = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
        timeout.CancelAfter(TimeSpan.FromSeconds(15));
        try
        {
            await process.WaitForExitAsync(timeout.Token).ConfigureAwait(false);
        }
        catch (OperationCanceledException) when (!cancellationToken.IsCancellationRequested)
        {
            try { process.Kill(entireProcessTree: true); } catch (InvalidOperationException) { }
            throw new LocalServiceException("project_identity_timeout", "Git 项目身份查询超过 15 秒。");
        }

        var output = (await stdout.ConfigureAwait(false)).Trim();
        var error = (await stderr.ConfigureAwait(false)).Trim();
        if (process.ExitCode != 0 || string.IsNullOrWhiteSpace(output))
            throw new LocalServiceException("project_identity_unavailable", error.Length == 0 ? "Git 未返回仓库 common-dir。" : error);
        return CanonicalPath(output);
    }

    private static string ProjectId(string identity) => Convert.ToHexString(SHA256.HashData(Encoding.UTF8.GetBytes(identity))).ToLowerInvariant();

    private static string RepositoryStateKey(string identity, string projectId)
    {
        var name = Path.GetFileName(identity);
        if (string.Equals(name, ".git", StringComparison.OrdinalIgnoreCase))
            name = Path.GetFileName(Path.GetDirectoryName(identity)) ?? string.Empty;
        name = Regex.Replace(name, "[^a-zA-Z0-9._-]+", "-").Trim('-', '.');
        if (name.Length == 0)
            name = "repo";
        return name + "-" + projectId[..12];
    }

    private static string CanonicalPath(string path) => Path.GetFullPath(path).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar).ToLowerInvariant();

    private static bool PathEquals(string left, string right) => string.Equals(CanonicalPath(left), CanonicalPath(right), StringComparison.OrdinalIgnoreCase);

    private static void VerifyProcessIdentity(ServiceProcessIdentity identity)
    {
        try
        {
            using var process = Process.GetProcessById(identity.Pid);
            if (process.HasExited)
                throw new LocalServiceException("stale_instance", "发现的服务进程已经退出。");
            var created = process.StartTime.ToUniversalTime().ToFileTimeUtc().ToString("x16", CultureInfo.InvariantCulture);
            var executable = process.MainModule?.FileName;
            if (!string.Equals(created, identity.Created, StringComparison.OrdinalIgnoreCase) || executable is null || !PathEquals(executable, identity.Executable))
                throw new LocalServiceException("stale_instance", "发现文件中的 PID、创建时间或可执行文件与当前进程不匹配。");
        }
        catch (LocalServiceException)
        {
            throw;
        }
        catch (Exception exception)
        {
            throw new LocalServiceException("stale_instance", $"无法核验发现的服务进程身份：{exception.Message}");
        }
    }

    private static void VerifyPrivateDirectory(string path)
    {
        if (!Directory.Exists(path) || (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0)
            throw new LocalServiceException("invalid_local_api_acl", "本地 API 目录不存在或是 reparse point。");
        var security = new DirectoryInfo(path).GetAccessControl(AccessControlSections.Owner | AccessControlSections.Access);
        VerifyPrivateAcl(security.GetOwner(typeof(SecurityIdentifier)) as SecurityIdentifier, security.AreAccessRulesProtected,
            security.GetAccessRules(includeExplicit: true, includeInherited: true, typeof(SecurityIdentifier)));
    }

    private static void VerifyPrivateFile(string path)
    {
        if (!File.Exists(path) || (File.GetAttributes(path) & FileAttributes.ReparsePoint) != 0)
            throw new LocalServiceException("invalid_local_api_acl", "本地 API 私有文件不存在或是 reparse point。");
        var security = new FileInfo(path).GetAccessControl(AccessControlSections.Owner | AccessControlSections.Access);
        VerifyPrivateAcl(security.GetOwner(typeof(SecurityIdentifier)) as SecurityIdentifier, security.AreAccessRulesProtected,
            security.GetAccessRules(includeExplicit: true, includeInherited: true, typeof(SecurityIdentifier)));
    }

    private static void VerifyPrivateAcl(SecurityIdentifier? owner, bool protectedDacl, AuthorizationRuleCollection rules)
    {
        var current = WindowsIdentity.GetCurrent().User;
        if (current is null || owner is null || !owner.Equals(current) || !protectedDacl || rules.Count != 1 || rules[0] is not FileSystemAccessRule rule ||
            rule.AccessControlType != AccessControlType.Allow || !rule.IdentityReference.Equals(current) ||
            rule.FileSystemRights != FileSystemRights.FullControl || rule.IsInherited || rule.InheritanceFlags != InheritanceFlags.None)
            throw new LocalServiceException("invalid_local_api_acl", "本地 API ACL 必须是当前 Windows SID 的单条受保护 FullControl 规则。");
    }

    private sealed class LocalServiceClient(HttpClient http, ServiceDiscovery discovery, string token, string homeDirectory) : IReadOnlyLocalService, IDisposable
    {
        private static readonly JsonSerializerOptions ClientJson = new() { PropertyNameCaseInsensitive = false };
        private ServiceIdentity? _identity;
        private bool _disposed;

        public ServiceIdentity Identity => _identity ?? throw new InvalidOperationException("Identity has not been verified.");
        public string HomeDirectory { get; } = homeDirectory;

        public async Task<ServiceIdentity> RefreshIdentityAsync(CancellationToken cancellationToken)
        {
            var identity = await GetAsync<ServiceIdentity>("/v1/identity", cancellationToken).ConfigureAwait(false);
            IdentityGuard.Verify(discovery, identity, discovery.ProjectId, discovery.RepositoryIdentity);
            if (_identity is not null && !IdentityGuard.SameInstance(_identity, identity))
                throw new LocalServiceException("identity_conflict", "实时服务实例身份在当前连接期间发生变化。");
            _identity = identity;
            return identity;
        }

        public async Task<TaskPage> ListTasksAsync(CancellationToken cancellationToken)
        {
            await RefreshIdentityAsync(cancellationToken).ConfigureAwait(false);
            var page = await GetProjectAsync<TaskPage>("/tasks?limit=500", cancellationToken).ConfigureAwait(false);
            TaskPageGuard.Verify(page, discovery.RepositoryIdentity);
            return page;
        }

        public async Task<TaskDetailPage> ShowTaskAsync(string taskId, CancellationToken cancellationToken)
        {
            await RefreshIdentityAsync(cancellationToken).ConfigureAwait(false);
            var page = await GetProjectAsync<TaskDetailPage>($"/tasks/{Uri.EscapeDataString(taskId)}", cancellationToken).ConfigureAwait(false);
            TaskPageGuard.Verify(page, discovery.RepositoryIdentity, taskId);
            return page;
        }

        private Task<T> GetProjectAsync<T>(string suffix, CancellationToken cancellationToken) => GetAsync<T>($"/v1/projects/{Uri.EscapeDataString(discovery.ProjectId)}{suffix}", cancellationToken);

        private async Task<T> GetAsync<T>(string path, CancellationToken cancellationToken)
        {
            ObjectDisposedException.ThrowIf(_disposed, this);
            using var request = new HttpRequestMessage(HttpMethod.Get, $"http://{discovery.Address}{path}");
            request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
            request.Headers.Accept.Add(new MediaTypeWithQualityHeaderValue("application/json"));
            request.Headers.Host = discovery.Address;
            using var requestTimeout = CancellationTokenSource.CreateLinkedTokenSource(cancellationToken);
            requestTimeout.CancelAfter(TimeSpan.FromSeconds(10));
            try
            {
                using var response = await http.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, requestTimeout.Token).ConfigureAwait(false);
                if (response.Content.Headers.ContentLength is > MaxResponseBytes)
                    throw new LocalServiceException("response_too_large", "本地 API 响应超过 32 MiB。");
                var bytes = await response.Content.ReadAsByteArrayAsync(requestTimeout.Token).ConfigureAwait(false);
                if (bytes.Length > MaxResponseBytes)
                    throw new LocalServiceException("response_too_large", "本地 API 响应超过 32 MiB。");
                if (!response.IsSuccessStatusCode)
                {
                    var apiError = JsonSerializer.Deserialize<ApiError>(bytes, ClientJson);
                    throw new LocalServiceException(apiError?.Code ?? "http_error", apiError?.Reason ?? $"HTTP {(int)response.StatusCode}");
                }
                return JsonSerializer.Deserialize<T>(bytes, ClientJson)
                    ?? throw new LocalServiceException("invalid_response", "本地 API 返回了空或无效 JSON 响应。");
            }
            catch (OperationCanceledException) when (!cancellationToken.IsCancellationRequested)
            {
                throw new LocalServiceException("request_timeout", "本地 API 读取超过 10 秒。");
            }
        }

        public void Dispose()
        {
            if (_disposed)
                return;
            _disposed = true;
            http.Dispose();
        }
    }

    private sealed record ApiError([property: JsonPropertyName("code")] string Code, [property: JsonPropertyName("reason")] string Reason);
}

public static class IdentityGuard
{
    public static void Verify(ServiceDiscovery discovery, ServiceIdentity identity, string expectedProjectId, string expectedRepositoryIdentity)
    {
        if (identity.Protocol != "steerline-client/v1" || identity.ProjectId != expectedProjectId ||
            !PathsEqual(identity.RepositoryIdentity, expectedRepositoryIdentity) || identity.ProjectId != discovery.ProjectId ||
            identity.InstanceId != discovery.InstanceId || !string.Equals(identity.Address, discovery.Address, StringComparison.Ordinal) ||
            identity.Process != discovery.Process)
            throw new LocalServiceException("identity_conflict", "实时服务身份与发现文件或所选项目不匹配。");
    }

    public static bool SameInstance(ServiceIdentity left, ServiceIdentity right) =>
        left.Protocol == right.Protocol && left.ProjectId == right.ProjectId && PathsEqual(left.RepositoryIdentity, right.RepositoryIdentity) &&
        left.InstanceId == right.InstanceId && left.Address == right.Address && left.Process == right.Process;

    public static bool RepositoryEquals(string left, string right) => PathsEqual(left, right);

    private static bool PathsEqual(string left, string right) =>
        string.Equals(Path.GetFullPath(left).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar),
            Path.GetFullPath(right).TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar), StringComparison.OrdinalIgnoreCase);
}

public sealed record ProjectConfiguration(
    string ConfigPath,
    string HomeDirectory,
    string RepositoryPath,
    string? RunDirectory,
    string GitExecutable,
    IReadOnlyList<string> GitArgs)
{
    public static ProjectConfiguration Read(string configPath)
    {
        var fullConfigPath = Path.GetFullPath(configPath);
        if (!File.Exists(fullConfigPath))
            throw new LocalServiceException("config_missing", "项目配置文件不存在。");
        using var document = JsonDocument.Parse(File.ReadAllBytes(fullConfigPath));
        var root = document.RootElement;
        var repository = ReadString(root, "repository");
        if (string.IsNullOrWhiteSpace(repository))
            throw new LocalServiceException("config_invalid", "项目配置缺少 repository 路径。");
        var gitExecutable = "git";
        var gitArgs = new List<string>();
        if (root.TryGetProperty("git", out var git) && git.ValueKind != JsonValueKind.Null)
        {
            if (git.ValueKind != JsonValueKind.Object)
                throw new LocalServiceException("config_invalid", "项目配置中的 Git 命令必须是对象。");
            if (git.TryGetProperty("executable", out var executable))
            {
                if (executable.ValueKind == JsonValueKind.String)
                    gitExecutable = executable.GetString()!;
                else if (executable.ValueKind != JsonValueKind.Null)
                    throw new LocalServiceException("config_invalid", "项目配置中的 Git executable 必须是字符串。");
            }
            if (git.TryGetProperty("args", out var args) && args.ValueKind != JsonValueKind.Null)
            {
                if (args.ValueKind != JsonValueKind.Array)
                    throw new LocalServiceException("config_invalid", "项目配置中的 Git args 必须是字符串数组。");
                foreach (var argument in args.EnumerateArray())
                {
                    if (argument.ValueKind == JsonValueKind.String)
                        gitArgs.Add(argument.GetString()!);
                    else if (argument.ValueKind == JsonValueKind.Null)
                        gitArgs.Add(string.Empty);
                    else
                        throw new LocalServiceException("config_invalid", "项目配置中的 Git args 必须是字符串数组。");
                }
            }
        }
        if (string.IsNullOrWhiteSpace(gitExecutable))
            throw new LocalServiceException("config_invalid", "项目配置缺少 Git executable。");
        var configDirectory = Path.GetDirectoryName(fullConfigPath)!;
        var repositoryPath = Path.GetFullPath(Path.IsPathRooted(repository) ? repository : Path.Combine(configDirectory, repository));
        if (!Directory.Exists(repositoryPath))
            throw new LocalServiceException("project_missing", "配置文件中的 repository 目录不存在。");
        var homeDirectory = Directory.GetParent(configDirectory)?.FullName ?? configDirectory;
        var runDirectory = ReadString(root, "run_dir");
        if (!string.IsNullOrWhiteSpace(runDirectory))
            runDirectory = Path.GetFullPath(Path.IsPathRooted(runDirectory) ? runDirectory : Path.Combine(configDirectory, runDirectory));
        return new ProjectConfiguration(fullConfigPath, Path.GetFullPath(homeDirectory), repositoryPath, runDirectory, gitExecutable, gitArgs);
    }

    internal static ProcessStartInfo CreateIdentityStartInfo(ProjectConfiguration configuration)
    {
        var startInfo = new ProcessStartInfo
        {
            FileName = configuration.GitExecutable,
            WorkingDirectory = configuration.RepositoryPath,
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            CreateNoWindow = true
        };
        foreach (var key in startInfo.Environment.Keys.Where(key => key.StartsWith("GIT_", StringComparison.OrdinalIgnoreCase)).ToArray())
            startInfo.Environment.Remove(key);
        foreach (var argument in configuration.GitArgs)
            startInfo.ArgumentList.Add(argument);
        startInfo.ArgumentList.Add("rev-parse");
        startInfo.ArgumentList.Add("--path-format=absolute");
        startInfo.ArgumentList.Add("--git-common-dir");
        return startInfo;
    }

    private static string? ReadString(JsonElement root, string property) =>
        root.TryGetProperty(property, out var value) && value.ValueKind == JsonValueKind.String ? value.GetString() : null;
}
