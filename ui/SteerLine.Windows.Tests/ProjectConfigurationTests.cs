using System.Text.Json;
using SteerLine.Windows.Core;

namespace SteerLine.Windows.Tests;

public sealed class ProjectConfigurationTests
{
    [Fact]
    public void UsesConfiguredGitExecutableAndArgumentsForRepositoryIdentity()
    {
        var root = Path.Combine(Path.GetTempPath(), "steerline-config-test-" + Guid.NewGuid().ToString("N"));
        var repository = Path.Combine(root, "repo");
        var configDirectory = Path.Combine(root, "home", "etc");
        Directory.CreateDirectory(repository);
        Directory.CreateDirectory(configDirectory);
        var configPath = Path.Combine(configDirectory, "project.json");
        const string executable = "C:\\Tools\\git-wrapper.exe";
        var configuredArgs = new[] { "--wrapper-mode", "inspect", "--preserve-arg" };

        try
        {
            File.WriteAllText(configPath, JsonSerializer.Serialize(new
            {
                repository,
                git = new { executable, args = configuredArgs }
            }));

            var configuration = ProjectConfiguration.Read(configPath);
            var startInfo = ProjectConfiguration.CreateIdentityStartInfo(configuration);

            Assert.Equal(executable, startInfo.FileName);
            Assert.Equal(Path.GetFullPath(repository), startInfo.WorkingDirectory);
            Assert.False(startInfo.UseShellExecute);
            Assert.Equal(configuredArgs.Concat(new[] { "rev-parse", "--path-format=absolute", "--git-common-dir" }), startInfo.ArgumentList);
        }
        finally
        {
            Directory.Delete(root, recursive: true);
        }
    }
}
