package scen

import (
  "database/sql"
  "math"
  "fmt"
)

func init() {
  // this registers your custom objective function with the master list of
  // all available objectives.  "myobjname" is the name by which you can
  // select/specify your objective from the scenario configuration file.
  // MyObjFuncName can is the name of the function that you choose below
  ObjFuncs["eg29"] = ObjEG29
}

// This function calculates your the objective value using the scenario
// configuration file parameters (i.e. in the scen arg) and a Cyclus database
// accessible via the db arg. The simid arg can be ignored unless the database
// might have more than one simulation in it.  The function then returns the
// objective value and an error if any occured.  This function can be named
// anything you like but should start with an upper case letter; be sure the
// ObjFuncs map assignment has this name on the right hand side.
//
// Documentation for the fields/info available in the scen struct can be found
// at https://godoc.org/github.com/rwcarlsen/cloudlus/scen#Scenario.
// Documentation for using the db object (e.g. running SQL queries) can be
// found at https://golang.org/pkg/database/sql/.
func ObjEG29(scen *Scenario, db *sql.DB, simid []byte) (float64, error) {

  q1 := `
  SELECT TOTAL(Value) FROM timeseriespower AS p
  JOIN agents AS a ON a.agentid=p.agentid AND a.simid=p.simid
  WHERE a.Prototype IN (?,?) AND p.simid=?
  `

  fmt.Printf("%v", q1)




  q2 := `
  SELECT TOTAL(Quantity) FROM explicitinventory AS p 
  JOIN agententry AS a ON a.agentid=p.agentid AND a.simid=p.simid
  WHERE a.Prototype IN (?,?) and p.simid=?
  `

  pwr_uox_power := 0.0
  err := db.QueryRow(q1, "LWR_UOX", "init_LWR_UOX", simid).Scan(&pwr_uox_power)
  if err != nil {
    return math.Inf(1), err
  }

  pwr_mox_power := 0.0
  err = db.QueryRow(q1, "PWR", "PWR", simid).Scan(&pwr_mox_power)
  if err != nil {
    return math.Inf(1), err
  }

  fbr_power := 0.0
  err = db.QueryRow(q1, "FBR_driver", "FBR", simid).Scan(&fbr_power)
  if err != nil {
    return math.Inf(1), err
  }

  power_tot:= pwr_uox_power + pwr_mox_power + fbr_power

  pu_stored := 0.0
  err_2 := db.QueryRow(q2, "Storage_E3_second", "Storage_E3_prime", simid).Scan(&pu_stored)
  if err_2 != nil {
    return math.Inf(1), err_2
  }

  // use data from the scenario configuration to calculate time integrated
  // installed capacity (i.e. cumulative energy capacity)
  builds := map[string][]Build{}
  for _, b := range scen.Builds {
    builds[b.Proto] = append(builds[b.Proto], b)
  }
  cap_tot := 0.0
  for t := 0; t < scen.SimDur; t++ {
    cap_tot += scen.PowerCap(builds, t)
  }


  return math.Pow(1 + pwr_uox_power/power_tot,4)* math.Pow(1 + cap_tot/power_tot,2) * (1 + pu_stored), nil

}
