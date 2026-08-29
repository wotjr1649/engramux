' Runs scripts/soak-sample.sh with no console window, for a Task Scheduler entry.
'
' It exists for exactly the reason spec 5.1 gives for the service being a GUI
' subsystem binary: Task Scheduler launching a console binary while the user is
' logged on creates a real, visible window. A sampler firing every 30 minutes
' for 72 hours would produce 144 of them. WScript.Shell.Run with intWindowStyle
' 0 is the one launch that creates none.
'
' It takes no arguments and hardcodes no machine: the repository is this file's
' own grandparent, and bash is found under %ProgramFiles%\Git. Both are checked,
' because a scheduled task that fails silently is a soak that records nothing.
Option Explicit

Dim fso, sh, repo, bash, cmd
Set fso = CreateObject("Scripting.FileSystemObject")
Set sh = CreateObject("WScript.Shell")

repo = fso.GetParentFolderName(fso.GetParentFolderName(WScript.ScriptFullName))
bash = sh.ExpandEnvironmentStrings("%ProgramFiles%") & "\Git\bin\bash.exe"

If Not fso.FileExists(bash) Then
	WScript.Echo "soak-sample-hidden: no bash at " & bash
	WScript.Quit 1
End If
If Not fso.FileExists(repo & "\scripts\soak-sample.sh") Then
	WScript.Echo "soak-sample-hidden: no soak-sample.sh under " & repo
	WScript.Quit 1
End If

' cd first, because soak-sample.sh derives its default log path from the
' repository root it is run inside.
cmd = """" & bash & """ -c ""cd '" & repo & "' && bash scripts/soak-sample.sh"""
WScript.Quit sh.Run(cmd, 0, True)
