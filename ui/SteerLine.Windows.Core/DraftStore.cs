using System.Text.Json;

namespace SteerLine.Windows.Core;

public interface IDraftStore
{
    string Get(string projectId, string targetId);
    void Set(string projectId, string targetId, string text);
    Task LoadAsync(string homeDirectory, CancellationToken cancellationToken);
    Task SaveAsync(CancellationToken cancellationToken);
}

public sealed class FileDraftStore : IDraftStore
{
    private readonly Dictionary<string, string> _drafts = new(StringComparer.Ordinal);
    private readonly SemaphoreSlim _saveGate = new(1, 1);
    private string? _path;

    public string Get(string projectId, string targetId) => _drafts.GetValueOrDefault(Key(projectId, targetId), string.Empty);

    public void Set(string projectId, string targetId, string text)
    {
        var key = Key(projectId, targetId);
        if (text.Length == 0)
            _drafts.Remove(key);
        else
            _drafts[key] = text;
    }

    public async Task LoadAsync(string homeDirectory, CancellationToken cancellationToken)
    {
        var nextPath = Path.Combine(homeDirectory, "ui", "drafts.json");
        if (_path is not null && !string.Equals(Path.GetFullPath(_path), Path.GetFullPath(nextPath), StringComparison.OrdinalIgnoreCase))
            _drafts.Clear();
        _path = nextPath;
        if (!File.Exists(_path))
            return;
        await using var stream = File.OpenRead(_path);
        var values = await JsonSerializer.DeserializeAsync<Dictionary<string, string>>(
            stream, cancellationToken: cancellationToken).ConfigureAwait(false);
        if (values is null)
            return;
        foreach (var item in values)
            _drafts[item.Key] = item.Value;
    }

    public async Task SaveAsync(CancellationToken cancellationToken)
    {
        if (_path is null)
            return;
        var path = _path;
        var snapshot = new Dictionary<string, string>(_drafts, StringComparer.Ordinal);
        await _saveGate.WaitAsync(cancellationToken).ConfigureAwait(false);
        try
        {
            Directory.CreateDirectory(Path.GetDirectoryName(path)!);
            var temp = path + ".tmp";
            await using (var stream = new FileStream(temp, FileMode.Create, FileAccess.Write, FileShare.None, 4096, useAsync: true))
                await JsonSerializer.SerializeAsync(stream, snapshot, cancellationToken: cancellationToken).ConfigureAwait(false);
            File.Move(temp, path, overwrite: true);
        }
        finally
        {
            _saveGate.Release();
        }
    }

    private static string Key(string projectId, string targetId) => projectId + "\n" + targetId;
}

public sealed class InMemoryDraftStore : IDraftStore
{
    private readonly Dictionary<(string ProjectId, string TargetId), string> _drafts = [];
    public string Get(string projectId, string targetId) => _drafts.GetValueOrDefault((projectId, targetId), string.Empty);
    public void Set(string projectId, string targetId, string text)
    {
        var key = (projectId, targetId);
        if (text.Length == 0) _drafts.Remove(key); else _drafts[key] = text;
    }
    public Task LoadAsync(string homeDirectory, CancellationToken cancellationToken) => Task.CompletedTask;
    public Task SaveAsync(CancellationToken cancellationToken) => Task.CompletedTask;
}
