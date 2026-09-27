//go:build windows

package web

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func pickDirectory(title string, allowNew bool) (string, error) {
	script := `
$ErrorActionPreference = "Stop"
Add-Type -ReferencedAssemblies System.Windows.Forms,System.Drawing -TypeDefinition @"
using System;
using System.Runtime.InteropServices;
using System.Windows.Forms;

[ComImport, Guid("DC1C5A9C-E88A-4dde-A5A1-60F82A20AEF7")]
class FileOpenDialogRCW {}

[ComImport, Guid("43826D1E-E718-42EE-BC55-A1E261C37BFE")]
[InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
interface IShellItem {
  void BindToHandler(IntPtr pbc, ref Guid bhid, ref Guid riid, out IntPtr ppv);
  void GetParent(out IShellItem ppsi);
  void GetDisplayName(uint sigdnName, [MarshalAs(UnmanagedType.LPWStr)] out string ppszName);
  void GetAttributes(uint sfgaoMask, out uint psfgaoAttribs);
  void Compare(IShellItem psi, uint hint, out int piOrder);
}

[ComImport, Guid("42f85136-db7e-439c-85f1-e4075d135fc8")]
[InterfaceType(ComInterfaceType.InterfaceIsIUnknown)]
interface IFileDialog {
  [PreserveSig] int Show(IntPtr parent);
  void SetFileTypes(uint cFileTypes, IntPtr rgFilterSpec);
  void SetFileTypeIndex(uint iFileType);
  void GetFileTypeIndex(out uint piFileType);
  void Advise(IntPtr pfde, out uint pdwCookie);
  void Unadvise(uint dwCookie);
  void SetOptions(uint fos);
  void GetOptions(out uint fos);
  void SetDefaultFolder(IShellItem psi);
  void SetFolder(IShellItem psi);
  void GetFolder(out IShellItem ppsi);
  void GetCurrentSelection(out IShellItem ppsi);
  void SetFileName([MarshalAs(UnmanagedType.LPWStr)] string pszName);
  void GetFileName([MarshalAs(UnmanagedType.LPWStr)] out string pszName);
  void SetTitle([MarshalAs(UnmanagedType.LPWStr)] string pszTitle);
  void SetOkButtonLabel([MarshalAs(UnmanagedType.LPWStr)] string pszText);
  void SetFileNameLabel([MarshalAs(UnmanagedType.LPWStr)] string pszLabel);
  void GetResult(out IShellItem ppsi);
  void AddPlace(IShellItem psi, int fdap);
  void SetDefaultExtension([MarshalAs(UnmanagedType.LPWStr)] string pszDefaultExtension);
  void Close(int hr);
  void SetClientGuid(ref Guid guid);
  void ClearClientData();
  void SetFilter(IntPtr pFilter);
}

public static class FolderPicker {
  public static string Pick(string title) {
    var dialog = (IFileDialog)new FileOpenDialogRCW();
    // FOS_PICKFOLDERS | FOS_FORCEFILESYSTEM | FOS_PATHMUSTEXIST
    dialog.SetOptions(0x20 | 0x40 | 0x800);
    dialog.SetTitle(title);
    var owner = new Form();
    owner.TopMost = true;
    owner.ShowInTaskbar = false;
    owner.StartPosition = FormStartPosition.CenterScreen;
    owner.Size = new System.Drawing.Size(0, 0);
    owner.Opacity = 0;
    owner.Show();
    int hr = dialog.Show(owner.Handle);
    owner.Close();
    if (hr == unchecked((int)0x800704C7)) return "";
    if (hr != 0) Marshal.ThrowExceptionForHR(hr);
    IShellItem item;
    dialog.GetResult(out item);
    string path;
    item.GetDisplayName(0x80058000, out path);
    return path ?? "";
  }
}
"@
[Console]::Out.Write([FolderPicker]::Pick($env:XISFITS_BROWSE_TITLE))
`
	cmd := exec.Command("powershell", "-NoProfile", "-STA", "-WindowStyle", "Hidden", "-Command", script)
	cmd.Env = append(os.Environ(), "XISFITS_BROWSE_TITLE="+title)
	_ = allowNew
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("open folder dialog: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
