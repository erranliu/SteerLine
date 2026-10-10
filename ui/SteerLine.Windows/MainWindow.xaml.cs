using Microsoft.UI.Xaml;
using Microsoft.UI.Xaml.Controls;
using SteerLine.Windows.Core;
using Windows.Storage.Pickers;
using WinRT.Interop;

namespace SteerLine.Windows;

public sealed partial class MainWindow : Window
{
    public MainWindowViewModel ViewModel { get; } = new(new LocalServiceFactory(), new FileDraftStore());

    public MainWindow()
    {
        InitializeComponent();
        Closed += OnWindowClosed;
    }

    private async void OnChooseConfigClick(object sender, RoutedEventArgs e)
    {
        var picker = new FileOpenPicker();
        picker.FileTypeFilter.Add(".json");
        InitializeWithWindow.Initialize(picker, WindowNative.GetWindowHandle(this));
        var file = await picker.PickSingleFileAsync();
        if (file is null)
            return;
        ConfigPathBox.Text = file.Path;
        try
        {
            ProjectPathBox.Text = ProjectConfiguration.Read(file.Path).RepositoryPath;
        }
        catch (Exception exception)
        {
            ViewModel.SetSelectionError(exception.Message);
        }
    }

    private async void OnChooseProjectClick(object sender, RoutedEventArgs e)
    {
        var picker = new FolderPicker();
        picker.FileTypeFilter.Add("*");
        InitializeWithWindow.Initialize(picker, WindowNative.GetWindowHandle(this));
        var folder = await picker.PickSingleFolderAsync();
        if (folder is not null)
            ProjectPathBox.Text = folder.Path;
    }

    private async void OnConnectClick(object sender, RoutedEventArgs e)
    {
        ConnectButton.IsEnabled = false;
        try
        {
            await ViewModel.ConnectAsync(ConfigPathBox.Text, ProjectPathBox.Text);
            DraftEditor.Text = ViewModel.CurrentDraft;
            TasksList.SelectedItem = ViewModel.SelectedTask;
        }
        finally
        {
            ConnectButton.IsEnabled = true;
        }
    }

    private async void OnRefreshClick(object sender, RoutedEventArgs e)
    {
        RefreshButton.IsEnabled = false;
        try
        {
            await ViewModel.RefreshAsync();
            DraftEditor.Text = ViewModel.CurrentDraft;
            TasksList.SelectedItem = ViewModel.SelectedTask;
        }
        finally
        {
            RefreshButton.IsEnabled = true;
        }
    }

    private async void OnTaskSelectionChanged(object sender, SelectionChangedEventArgs e)
    {
        var selection = ViewModel.SelectTaskAsync(TasksList.SelectedItem as TaskSummary);
        DraftEditor.Text = ViewModel.CurrentDraft;
        await selection;
    }

    private void OnDraftTextChanged(object sender, TextChangedEventArgs e) => ViewModel.UpdateDraft(DraftEditor.Text);

    private void OnRecordIntentClick(object sender, RoutedEventArgs e) => ViewModel.RecordLocalIntent();

    private async void OnWindowClosed(object sender, WindowEventArgs args) => await ViewModel.DisposeAsync();
}
