// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package instrumentationnew

import (
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

var ErrEnvFromValueFrom = errors.New("container env var is sourced via ValueFrom and cannot be mutated by the injector")

type envValueFromError struct{ name string }

func (e envValueFromError) Error() string {
	return "the container defines env var value via ValueFrom, envVar: " + e.name
}

func (e envValueFromError) Unwrap() error { return ErrEnvFromValueFrom }

func errEnvNotMutable(name string) error { return envValueFromError{name: name} }

type envOpKind int

const (
	envSetIfAbsent envOpKind = iota
	envSetIfAllAbsent
	envRejectValueFrom
	envAppend
	envPrepend
	envResourceAttrs
	envMoveToEnd
)

type envPosition int

const (
	envAppendPosition envPosition = iota
	envPrependPosition
)

type envOp struct {
	kind      envOpKind
	env       corev1.EnvVar
	envs      []corev1.EnvVar
	blockers  []string
	name      string
	value     string
	separator string
	position  envPosition
	resource  ResourceAttributes
}

type ContainerPatch struct {
	envOps       []envOp
	VolumeMounts []corev1.VolumeMount
}

func (c *ContainerPatch) AddEnv(name, value string) {
	c.AddEnvVar(corev1.EnvVar{Name: name, Value: value})
}

func (c *ContainerPatch) AddEnvFirst(name string, source *corev1.EnvVarSource) {
	c.envOps = append(c.envOps, envOp{
		kind: envSetIfAbsent,
		env: corev1.EnvVar{
			Name:      name,
			ValueFrom: source,
		},
		position: envPrependPosition,
	})
}

func (c *ContainerPatch) AddEnvFromField(name, fieldPath string) {
	c.AddEnvVar(corev1.EnvVar{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: fieldPath},
		},
	})
}

func (c *ContainerPatch) AddEnvVar(env corev1.EnvVar) {
	c.envOps = append(c.envOps, envOp{kind: envSetIfAbsent, env: env})
}

func (c *ContainerPatch) AddEnvVars(envs ...corev1.EnvVar) {
	for _, env := range envs {
		c.AddEnvVar(env)
	}
}

func (c *ContainerPatch) AddEnvVarsIfAllAbsent(blockers []string, envs ...corev1.EnvVar) {
	c.envOps = append(c.envOps, envOp{
		kind:     envSetIfAllAbsent,
		blockers: append([]string(nil), blockers...),
		envs:     append([]corev1.EnvVar(nil), envs...),
	})
}

func (c *ContainerPatch) RejectValueFrom(names ...string) {
	for _, name := range names {
		c.envOps = append(c.envOps, envOp{kind: envRejectValueFrom, name: name})
	}
}

func (c *ContainerPatch) AppendEnvValue(name, addition string) {
	c.AppendEnvValueWithSep(name, addition, "")
}

func (c *ContainerPatch) AppendEnvValueWithSep(name, addition, separator string) {
	c.envOps = append(c.envOps, envOp{kind: envAppend, name: name, value: addition, separator: separator})
}

func (c *ContainerPatch) PrependEnvValue(name, addition string) {
	c.PrependEnvValueWithSep(name, addition, "")
}

func (c *ContainerPatch) PrependEnvValueWithSep(name, addition, separator string) {
	c.envOps = append(c.envOps, envOp{kind: envPrepend, name: name, value: addition, separator: separator})
}

func (c *ContainerPatch) AddResourceAttributes(attrs ResourceAttributes) {
	c.envOps = append(c.envOps, envOp{kind: envResourceAttrs, resource: attrs})
}

func (c *ContainerPatch) MoveEnvToEnd(name string) {
	c.envOps = append(c.envOps, envOp{kind: envMoveToEnd, name: name})
}

func (c *ContainerPatch) AddVolumeMount(mount corev1.VolumeMount) {
	c.VolumeMounts = append(c.VolumeMounts, mount)
}

func (c *ContainerPatch) merge(src ContainerPatch) {
	c.envOps = append(c.envOps, src.envOps...)
	c.VolumeMounts = append(c.VolumeMounts, src.VolumeMounts...)
}

func applyEnvOps(existing []corev1.EnvVar, ops []envOp) ([]corev1.EnvVar, error) {
	em := newEnvMap(existing)
	for _, op := range ops {
		switch op.kind {
		case envSetIfAbsent:
			em.setIfAbsent(op.env, op.position)
		case envSetIfAllAbsent:
			if em.hasAny(op.blockers...) {
				continue
			}
			for _, env := range op.envs {
				em.setIfAbsent(env, envAppendPosition)
			}
		case envRejectValueFrom:
			if err := em.rejectValueFrom(op.name); err != nil {
				return nil, err
			}
		case envAppend:
			if err := em.append(op.name, op.value, op.separator); err != nil {
				return nil, err
			}
		case envPrepend:
			if err := em.prepend(op.name, op.value, op.separator); err != nil {
				return nil, err
			}
		case envResourceAttrs:
			if err := em.appendResourceAttributes(op.resource); err != nil {
				return nil, err
			}
		case envMoveToEnd:
			em.moveToEnd(op.name)
		default:
			return nil, fmt.Errorf("unknown env op kind %d", op.kind)
		}
	}
	return em.slice(), nil
}

type envMap struct {
	byName map[string]corev1.EnvVar
	order  []string
}

func newEnvMap(env []corev1.EnvVar) *envMap {
	m := &envMap{
		byName: make(map[string]corev1.EnvVar, len(env)),
		order:  make([]string, 0, len(env)),
	}
	for _, e := range env {
		if _, dup := m.byName[e.Name]; dup {
			continue
		}
		m.byName[e.Name] = e
		m.order = append(m.order, e.Name)
	}
	return m
}

func (m *envMap) has(name string) bool {
	_, ok := m.byName[name]
	return ok
}

func (m *envMap) hasAny(names ...string) bool {
	for _, name := range names {
		if m.has(name) {
			return true
		}
	}
	return false
}

func (m *envMap) setIfAbsent(env corev1.EnvVar, position envPosition) {
	if m.has(env.Name) {
		return
	}
	m.byName[env.Name] = env
	if position == envPrependPosition {
		m.order = append([]string{env.Name}, m.order...)
		return
	}
	m.order = append(m.order, env.Name)
}

func (m *envMap) set(env corev1.EnvVar) {
	if !m.has(env.Name) {
		m.order = append(m.order, env.Name)
	}
	m.byName[env.Name] = env
}

func (m *envMap) rejectValueFrom(name string) error {
	cur, ok := m.byName[name]
	if !ok || cur.ValueFrom == nil {
		return nil
	}
	return errEnvNotMutable(name)
}

func (m *envMap) append(name, addition, separator string) error {
	cur, ok := m.byName[name]
	if !ok {
		m.set(corev1.EnvVar{Name: name, Value: addition})
		return nil
	}
	if cur.ValueFrom != nil {
		return errEnvNotMutable(name)
	}
	if cur.Value != "" && addition != "" && separator != "" && !strings.HasSuffix(cur.Value, separator) {
		cur.Value += separator
	}
	cur.Value += addition
	m.byName[name] = cur
	return nil
}

func (m *envMap) prepend(name, addition, separator string) error {
	cur, ok := m.byName[name]
	if !ok {
		m.set(corev1.EnvVar{Name: name, Value: addition})
		return nil
	}
	if cur.ValueFrom != nil {
		return errEnvNotMutable(name)
	}
	sep := ""
	if cur.Value != "" && addition != "" && separator != "" && !strings.HasPrefix(cur.Value, separator) {
		sep = separator
	}
	cur.Value = addition + sep + cur.Value
	m.byName[name] = cur
	return nil
}

func (m *envMap) appendResourceAttributes(attrs ResourceAttributes) error {
	const name = "OTEL_RESOURCE_ATTRIBUTES"

	existing := ""
	if cur, ok := m.byName[name]; ok {
		if cur.ValueFrom != nil {
			return errEnvNotMutable(name)
		}
		existing = cur.Value
	}

	built := attrs.Build(existing)
	if built == "" {
		m.moveToEnd(name)
		return nil
	}
	if err := m.append(name, built, ","); err != nil {
		return err
	}
	m.moveToEnd(name)
	return nil
}

func (m *envMap) moveToEnd(name string) {
	idx := -1
	for i, current := range m.order {
		if current == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		return
	}
	m.order = append(m.order[:idx], m.order[idx+1:]...)
	m.order = append(m.order, name)
}

func (m *envMap) slice() []corev1.EnvVar {
	out := make([]corev1.EnvVar, 0, len(m.order))
	for _, name := range m.order {
		out = append(out, m.byName[name])
	}
	return out
}
