#!/bin/bash
tmux -L vhs_demo kill-session -t demo 2>/dev/null || true
tmux -L vhs_demo new-session -s demo -d 'bash'
tmux -L vhs_demo set-option -g default-terminal "xterm-256color"
tmux -L vhs_demo set-option -ga terminal-overrides ",xterm-256color*:Tc"
tmux -L vhs_demo set-option -g status on
tmux -L vhs_demo set-option -g status-position bottom
tmux -L vhs_demo set-option -g status-style "bg=default"
tmux -L vhs_demo set-option -g status-left-length 200
tmux -L vhs_demo set-option -g status-right ""
tmux -L vhs_demo set-option -g window-status-current-format ""
tmux -L vhs_demo set-option -g window-status-format ""
tmux -L vhs_demo set-option -g status-left ""
tmux -L vhs_demo attach -t demo
