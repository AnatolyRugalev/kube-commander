package cfg

import (
	"errors"
	"github.com/spf13/cobra"
	flag "github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type Pkg struct {
	PersistentFlags FlagsDeclaration
	LocalFlags      FlagsDeclaration
	Struct          interface{}
}

type FlagsDeclaration map[string]struct {
	Def   interface{}
	Usage string
	Env   string
}

var pkgs []*Pkg
var cmds []*cobra.Command

type flagSet struct {
	*flag.FlagSet
}

func AddPkg(pkg ...*Pkg) {
	pkgs = append(pkgs, pkg...)
}

func AddCommand(cmd ...*cobra.Command) {
	cmds = append(cmds, cmd...)
}

func Setup(rootCmd *cobra.Command) error {
	rootCmd.AddCommand(cmds...)

	pFlags := &flagSet{rootCmd.PersistentFlags()}
	lFlags := &flagSet{rootCmd.LocalFlags()}
	for _, pkg := range pkgs {
		if err := pFlags.addFlags(pkg.PersistentFlags); err != nil {
			return err
		}
		if err := lFlags.addFlags(pkg.LocalFlags); err != nil {
			return err
		}
	}
	return nil
}

func Apply() error {
	for _, pkg := range pkgs {
		if err := pkg.applyFlags(); err != nil {
			return err
		}
	}
	return nil
}

func (fs *flagSet) addFlags(flags FlagsDeclaration) error {
	for key, f := range flags {
		var err error
		switch v := f.Def.(type) {
		case string:
			err = fs.string(key, v, f.Usage, f.Env)
		case int:
			err = fs.int(key, v, f.Usage, f.Env)
		case bool:
			err = fs.bool(key, v, f.Usage, f.Env)
		default:
			err = errors.New("invalid type of " + key + " configuration key")
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (fs *flagSet) string(key string, def string, usage string, envKey string) error {
	fs.String(key, def, usage)
	err := viper.BindPFlag(key, fs.Lookup(key))
	if err != nil {
		return err
	}
	return viper.BindEnv(key, envKey)
}

func (fs *flagSet) int(key string, def int, usage string, envKey string) error {
	fs.Int(key, def, usage)
	err := viper.BindPFlag(key, fs.Lookup(key))
	if err != nil {
		return err
	}
	return viper.BindEnv(key, envKey)
}

func (fs *flagSet) bool(key string, def bool, usage string, envKey string) error {
	fs.Bool(key, def, usage)
	err := viper.BindPFlag(key, fs.Lookup(key))
	if err != nil {
		return err
	}
	return viper.BindEnv(key, envKey)
}

func (pkg *Pkg) applyFlags() error {
	if pkg.Struct == nil {
		return nil
	}
	return viper.Unmarshal(pkg.Struct)
}
