namespace SteerLine.Windows.Core;

/// <summary>Keeps the last window alive until its local draft flush succeeds.</summary>
public sealed class WindowCloseGuard
{
    private bool _saving;
    public bool IsSaving => _saving;
    public bool CanClose { get; private set; }

    public async Task<bool> PrepareAsync(Func<Task> save, Action<Exception> showError)
    {
        if (CanClose) return true;
        if (_saving) return false;
        _saving = true;
        try
        {
            await save();
            CanClose = true;
            return true;
        }
        catch (Exception exception)
        {
            showError(exception);
            return false;
        }
        finally
        {
            _saving = false;
        }
    }
}
