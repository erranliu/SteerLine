using SteerLine.Windows.Core;

namespace SteerLine.Windows.Tests;

public sealed class WindowCloseGuardTests
{
    [Fact]
    public async Task PendingSavePreventsCloseAndRepeatedRequestsDoNotStartAnotherSave()
    {
        var guard = new WindowCloseGuard();
        var persisted = new TaskCompletionSource(TaskCreationOptions.RunContinuationsAsynchronously);
        var calls = 0;
        Task Save() { calls++; return persisted.Task; }
        var closing = guard.PrepareAsync(Save, _ => Assert.Fail("Unexpected save failure"));

        Assert.False(guard.CanClose);
        Assert.True(guard.IsSaving);
        Assert.False(closing.IsCompleted);
        Assert.False(await guard.PrepareAsync(Save, _ => Assert.Fail("Unexpected save failure")));
        Assert.Equal(1, calls);

        persisted.SetResult();
        Assert.True(await closing);
        Assert.True(guard.CanClose);
        Assert.False(guard.IsSaving);
        Assert.True(await guard.PrepareAsync(Save, _ => Assert.Fail("Unexpected save failure")));
        Assert.Equal(1, calls);
    }

    [Fact]
    public async Task FailedSaveKeepsWindowOpenReportsErrorAndAllowsRetry()
    {
        var guard = new WindowCloseGuard();
        Exception? reported = null;
        var error = new IOException("disk full");

        Assert.False(await guard.PrepareAsync(() => Task.FromException(error), e => reported = e));
        Assert.Same(error, reported);
        Assert.False(guard.CanClose);
        Assert.False(guard.IsSaving);
        Assert.True(await guard.PrepareAsync(() => Task.CompletedTask, _ => Assert.Fail("Retry failed")));
        Assert.True(guard.CanClose);
    }
}
