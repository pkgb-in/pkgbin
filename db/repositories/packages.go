package repositories

import (
	"fmt"

	"github.com/pkgb-in/pkgbin/db/models"
	"github.com/pkgb-in/pkgbin/initializers"
	"gorm.io/gorm"
)

type PackageRepository struct {
	db *gorm.DB // Example: if using GORM for database operations
}

// func NewPackageRepository(db *gorm.DB) *PackageRepository {
// 	return &PackageRepository{db: db}
// }

var PackageRepo *PackageRepository

func InitPackageRepository() {
	if initializers.DB == nil {
		panic("InitPackageRepository: database is nil; ensure InitDatabase succeeded")
	}
	PackageRepo = &PackageRepository{db: initializers.DB}
	fmt.Println("Package Repository initialized")
}

func (r *PackageRepository) GetPackageByName(name string) (models.Package, error) {
	var pkg models.Package
	result := r.db.First(&pkg, "name = ?", name)
	return pkg, result.Error
}

func (r *PackageRepository) CreatePackage(pkg *models.Package) error {
	result := r.db.Create(pkg)
	return result.Error
}

func (r *PackageRepository) UpdatePackageAccess(name string, hit bool) error {
	// Call the Postgres function; SELECT is the correct way to invoke a FUNCTION
	// Use Raw+Rows to execute without needing to scan a result
	rows, err := r.db.Raw("SELECT record_package_access(?, ?)", name, hit).Rows()
	if err != nil {
		return err
	}
	defer rows.Close()
	return nil
}

// ListPackagesPaginated returns a paginated list of packages and the total count
func (r *PackageRepository) ListPackagesPaginated(page, pageSize int) ([]models.Package, int, error) {
	var pkgs []models.Package
	var total int64
	r.db.Model(&models.Package{}).Count(&total)
	offset := (page - 1) * pageSize
	result := r.db.Order("id").Limit(pageSize).Offset(offset).Find(&pkgs)
	return pkgs, int(total), result.Error
}

// ListPackagesByNamePaginated returns a paginated list of packages filtered by name and the total count
func (r *PackageRepository) ListPackagesByNamePaginated(name string, page, pageSize int) ([]models.Package, int, error) {
	var pkgs []models.Package
	var total int64
	query := r.db.Model(&models.Package{}).Where("name ILIKE ?", "%"+name+"%")
	query.Count(&total)
	offset := (page - 1) * pageSize
	result := query.Order("id").Limit(pageSize).Offset(offset).Find(&pkgs)
	return pkgs, int(total), result.Error
}

// DeletePackagesByNames deletes packages from the database by their names
func (r *PackageRepository) DeletePackagesByNames(names []string) error {
	result := r.db.Where("name IN ?", names).Delete(&models.Package{})
	return result.Error
}

// GetTotalPackagesServed returns the total number of packages served (sum of cache hits and misses)
func (r *PackageRepository) GetTotalPackagesServed() (int64, error) {
	var total struct {
		Total int64
	}
	result := r.db.Model(&models.Package{}).Select("COALESCE(SUM(cache_hit + cache_miss), 0) as total").Scan(&total)
	return total.Total, result.Error
}

// TruncatePackagesTable removes all records from the packages table
func (r *PackageRepository) TruncatePackagesTable() error {
	result := r.db.Exec("TRUNCATE TABLE packages RESTART IDENTITY CASCADE")
	return result.Error
}

// ResetSequence resets the packages table ID sequence to 1
func (r *PackageRepository) ResetSequence() error {
	result := r.db.Exec("ALTER SEQUENCE packages_id_seq RESTART WITH 1")
	return result.Error
}
