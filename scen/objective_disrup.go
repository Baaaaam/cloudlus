package scen

import (
	"errors"
	"fmt"
	"math"
	"sync"
)

type Disruption struct {
	// Time is the time step on which the facility shutdown disruption occurs.
	Time int
	// KillProto is the prototype for which all facilities will be shut down
	// by the given time.
	KillProto string
	// BuildProto is the prototype of which to build a single new instance at
	// the given time.
	BuildProto string
	// Prob holds the probability that the disruption will happen at a
	// particular time.  This is ignored in disrup-single mode.
	Prob float64
	// KnownBest holds the objective value for the best known deployment
	// schedule for the scenario for with a priori knowledge of this
	// particular disruption always occuring.  This is only used in
	// disrup-multi-lin mode.
	KnownBest float64
}

func disrupSingleMode(s *Scenario, obj ObjExecFunc) (float64, error) {
	idisrup := s.CustomConfig["disrup-single"].(map[string]interface{})
	disrup := Disruption{}

	if t, ok := idisrup["Time"]; ok {
		disrup.Time = int(t.(float64))
	}

	if proto, ok := idisrup["KillProto"]; ok {
		disrup.KillProto = proto.(string)
	}

	if proto, ok := idisrup["BuildProto"]; ok {
		disrup.BuildProto = proto.(string)
	}

	if prob, ok := idisrup["Prob"]; ok {
		disrup.Prob = prob.(float64)
	}

	// set separations plant to die disruption time.
	clone := s.Clone()
	clone.Builds = append(clone.Builds, buildsForDisrup(clone, disrup)...)
	if disrup.Time >= 0 {
		for i, b := range clone.Builds {
			clone.Builds[i] = modForDisrup(clone, disrup, b)
		}
	}

	return obj(clone)
}

func buildsForDisrup(s *Scenario, disrup Disruption) []Build {
	if disrup.Time < 0 || disrup.BuildProto == "" {
		return []Build{}
	}

	b := Build{
		Time:  disrup.Time,
		N:     1,
		Proto: disrup.BuildProto,
	}

	for _, fac := range s.Facs {
		if fac.Proto == b.Proto {
			b.fac = fac
			return []Build{b}
		}
	}
	panic("prototype " + b.Proto + " not found")
}

func modForDisrup(s *Scenario, disrup Disruption, b Build) Build {
	if disrup.Time < 0 {
		return b
	} else if b.Proto != disrup.KillProto {
		return b
	}

	b.Life = disrup.Time - b.Time
	return b
}

// disrupModeLin is the same as disrupMode except it performs the weighted
// linear combination of each sub objective with the know best for that
// disruption time to compute the final sub objectives that are then combined.
func disrupModeLin(s *Scenario, obj ObjExecFunc) (float64, error) {
	idisrup := s.CustomConfig["disrup-multi"].([]interface{})
	disrup := make([]Disruption, len(idisrup))
	for i, td := range idisrup {
		m := td.(map[string]interface{})
		d := Disruption{}

		if t, ok := m["Time"]; ok {
			d.Time = int(t.(float64))
		}

		if proto, ok := m["KillProto"]; ok {
			d.KillProto = proto.(string)
		}

		if prob, ok := m["Prob"]; ok {
			d.Prob = prob.(float64)
		}

		if v, ok := m["KnownBest"]; ok {
			d.KnownBest = v.(float64)
		} else {
			return math.Inf(1), errors.New("disrup-multi-lin needs KnownBest parameters set in custom disruption config")
		}

		disrup[i] = d
	}

	// fire off concurrent sub-simulation objective evaluations
	var wg sync.WaitGroup
	wg.Add(len(disrup))
	subobjs := make([]float64, len(disrup))
	var errinner error
	for i, d := range disrup {
		// set separations plant to die disruption time.
		clone := s.Clone()
		clone.Builds = append(clone.Builds, buildsForDisrup(clone, d)...)
		if d.Time >= 0 {
			for i, b := range clone.Builds {
				clone.Builds[i] = modForDisrup(clone, d, b)
			}
		}

		go func(i int, s *Scenario) {
			defer wg.Done()
			val, err := obj(s)
			if err != nil {
				errinner = err
				val = math.Inf(1)
			}
			subobjs[i] = val
		}(i, clone)
	}

	wg.Wait()
	if errinner != nil {
		return math.Inf(1), fmt.Errorf("remote sub-simulation execution failed: %v", errinner)
	}

	// compute aggregate objective
	objval := 0.0
	for i := range subobjs {
		wPre := float64(disrup[i].Time) / float64(s.SimDur)
		wPost := 1 - wPre
		subobj := wPre*subobjs[i] + wPost*disrup[i].KnownBest
		objval += disrup[i].Prob * subobj
	}
	return objval, nil
}

func disrupMode(s *Scenario, obj ObjExecFunc) (float64, error) {
	idisrup := s.CustomConfig["disrup-multi"].([]interface{})
	disrup := make([]Disruption, len(idisrup))
	for i, td := range idisrup {
		m := td.(map[string]interface{})
		d := Disruption{}

		if t, ok := m["Time"]; ok {
			d.Time = int(t.(float64))
		}

		if proto, ok := m["KillProto"]; ok {
			d.KillProto = proto.(string)
		}

		if prob, ok := m["Prob"]; ok {
			d.Prob = prob.(float64)
		}

		disrup[i] = d
	}

	// fire off concurrent sub-simulation objective evaluations
	var wg sync.WaitGroup
	wg.Add(len(disrup))
	subobjs := make([]float64, len(disrup))
	var errinner error
	for i, d := range disrup {
		// set separations plant to die disruption time.
		clone := s.Clone()
		clone.Builds = append(clone.Builds, buildsForDisrup(clone, d)...)
		if d.Time >= 0 {
			for i, b := range clone.Builds {
				clone.Builds[i] = modForDisrup(clone, d, b)
			}
		}

		go func(i int, s *Scenario) {
			defer wg.Done()
			val, err := obj(s)
			if err != nil {
				errinner = err
				val = math.Inf(1)
			}
			subobjs[i] = val
		}(i, clone)
	}

	wg.Wait()
	if errinner != nil {
		return math.Inf(1), fmt.Errorf("remote sub-simulation execution failed: %v", errinner)
	}

	// compute aggregate objective
	objval := 0.0
	for i := range subobjs {
		objval += disrup[i].Prob * subobjs[i]
	}
	return objval, nil
}
