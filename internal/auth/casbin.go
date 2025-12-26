package auth

import (
	"log"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"gorm.io/gorm"
)

// InitCasbin defines the RBAC model and initializes the enforcer with GORM adapter
func InitCasbin(db *gorm.DB) (*casbin.Enforcer, error) {
	// 1. Initialize GORM adapter (creates casbin_rule table)
	adapter, err := gormadapter.NewAdapterByDB(db)
	if err != nil {
		return nil, err
	}

	// 2. Define RBAC Model in string format
	// r = request (who, what, how)
	// p = policy (who, what, how)
	// g = grouping (role hierarchy)
	// m = matcher (how to match request to policy)
	text := `
		[request_definition]
		r = sub, obj, act

		[policy_definition]
		p = sub, obj, act

		[role_definition]
		g = _, _

		[policy_effect]
		e = some(where (p.eft == allow))

		[matchers]
		m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && regexMatch(r.act, p.act)
	`
	// keyMatch2 supports URL parameters like /users/:id/resource

	m, err := model.NewModelFromString(text)
	if err != nil {
		return nil, err
	}

	// 3. Create Enforcer
	enforcer, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		return nil, err
	}

	// 4. Load policy from database
	if err := enforcer.LoadPolicy(); err != nil {
		return nil, err
	}

	// 5. Initialize default policies if empty
	// Check if there are any policies (excluding grouping policies)
	policies, _ := enforcer.GetPolicy()
	if len(policies) == 0 {
		log.Println("Casbin: No policies found, initializing default admin policy...")
		
		// p, admin, /api/*, (GET)|(POST)|(PUT)|(DELETE)
		// This user 'admin' can access any path starting with /api/ using any common method
		_, err := enforcer.AddPolicy("admin", "/api/*", "(GET)|(POST)|(PUT)|(DELETE)")
		if err != nil {
			log.Printf("Failed to add default policy: %v", err)
		} else {
			// Save the new policy back to DB
			if err := enforcer.SavePolicy(); err != nil {
				log.Printf("Failed to save default policy: %v", err)
			} else {
				log.Println("Casbin: Default admin policy initialized.")
			}
		}
	}

	log.Println("Casbin initialized successfully")
	return enforcer, nil
}
